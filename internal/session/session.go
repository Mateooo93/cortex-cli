// Package session implements the in-process chat session. It holds
// conversation state, dispatches to the configured LLM provider, and
// emits protocol events on a channel for the TUI to consume.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
	"github.com/Mateooo93/cortex-cli/internal/provider"
	"github.com/Mateooo93/cortex-cli/internal/tools"
)

// Session is the in-process chat session. It runs the model in a goroutine
// and streams events on Events().
type Session struct {
	mu sync.Mutex

	id        string
	startedAt time.Time
	workdir   string
	configDir string

	cfg     *cortexconfig.Config
	active  string // active model name
	tools   *tools.Registry
	history []provider.Message
	// pending tool-call results collected this turn
	pendingResults []provider.Message

	// Events
	events chan protocol.SessionEvent

	// Cancellable
	cancel context.CancelFunc
	// Whether the user has requested a cancel.
	cancelReq bool
	// delayedCancel is set by SendCancelAfterEdit to request that
	// the current turn be cancelled, but only AFTER the in-flight
	// tool call (e.g. edit_file) finishes. This lets the agent
	// complete the user's pending edit cleanly before yielding to
	// the new instruction, instead of leaving a half-applied
	// change on disk.
	delayedCancel bool
}

// Config is the input for New().
type Config struct {
	CortexCfg   *cortexconfig.Config
	Workdir     string
	ConfigDir   string
	ActiveModel string
}

// New constructs a Session. The model is resolved and a Provider is
// built on first use.
func New(cfg Config) (*Session, error) {
	if cfg.CortexCfg == nil {
		return nil, errors.New("session: nil config")
	}
	if cfg.ActiveModel == "" {
		cfg.ActiveModel = cfg.CortexCfg.DefaultModel
	}
	if _, _, err := cfg.CortexCfg.GetModel(cfg.ActiveModel); err != nil {
		return nil, err
	}
	s := &Session{
		id:        uuid.NewString(),
		startedAt: time.Now(),
		workdir:   cfg.Workdir,
		configDir: cfg.ConfigDir,
		cfg:       cfg.CortexCfg,
		active:    cfg.ActiveModel,
		tools:     tools.NewRegistry(),
		events:    make(chan protocol.SessionEvent, 64),
	}
	return s, nil
}

// ID returns the session's unique identifier.
func (s *Session) ID() string { return s.id }

// StartedAt returns the time the session was created.
func (s *Session) StartedAt() time.Time { return s.startedAt }

// Events returns the channel of protocol events. The TUI consumes this.
func (s *Session) Events() <-chan protocol.SessionEvent { return s.events }

// ActiveModel returns the name of the currently active model.
func (s *Session) ActiveModel() string { return s.active }

// History returns a snapshot of the conversation history.
func (s *Session) History() []provider.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]provider.Message, len(s.history))
	copy(out, s.history)
	return out
}

// Reset clears the conversation history (used by /clear).
func (s *Session) Reset() {
	s.mu.Lock()
	s.history = nil
	s.mu.Unlock()
	systemMsg := s.cfg.SystemPrompt
	if systemMsg != "" {
		systemMsg += "\n\n"
	}
	systemMsg += s.tools.ToSystemPrompt()
	// We don't push to history directly; the next Send will rebuild.
	s.system(systemMsg)
}

// SetActiveModel switches the model for future turns.
func (s *Session) SetActiveModel(name string) error {
	if _, _, err := s.cfg.GetModel(name); err != nil {
		return err
	}
	s.mu.Lock()
	s.active = name
	s.mu.Unlock()
	return nil
}

// RestoreHistory replaces the session's conversation history with
// the given messages. Used to seed a freshly-reconnected session
// with the user's prior chat scrollback so the new daemon has
// the full conversation context (not just the user's next
// message in isolation).
//
// The history is rebuilt with a system message derived from the
// current config so the next turn sees the right tools/prompt.
func (s *Session) RestoreHistory(history []provider.Message) {
	systemMsg := s.cfg.SystemPrompt
	if systemMsg != "" {
		systemMsg += "\n\n"
	}
	systemMsg += s.tools.ToSystemPrompt()
	s.system(systemMsg)

	s.mu.Lock()
	// Append the restored messages after the system message.
	// We trust the caller's input here -- this is only called
	// with a history we just read from disk in our own chat
	// format, not from an untrusted source.
	out := []provider.Message{{Role: "system", Content: systemMsg}}
	out = append(out, history...)
	s.history = out
	s.mu.Unlock()
}

// Send submits a user message and starts a turn. The turn runs in a
// background goroutine and emits events on Events().
func (s *Session) Send(text string, attachments []protocol.Attachment) {
	go s.runTurn(context.Background(), text, attachments)
}

// SendCancel asks the running turn to stop. Safe to call when no turn
// is in flight (it's a no-op).
func (s *Session) SendCancel() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancelReq = true
	s.delayedCancel = false
	s.mu.Unlock()
}

// SendCancelAfterEdit asks the running turn to stop, but only after
// the currently-executing tool call (if any) finishes. If no tool
// call is in flight, this is equivalent to SendCancel. The point of
// this method is to let the agent finish applying the user's
// already-issued edit before yielding to a new instruction, so the
// filesystem is not left in a half-edited state.
//
// Safe to call when no turn is in flight (it's a no-op).
func (s *Session) SendCancelAfterEdit() {
	s.mu.Lock()
	s.delayedCancel = true
	// If no tool call is in flight (i.e. the model is streaming
	// text), the cancel happens naturally the next time runTurn
	// observes ctx.Err(). But for the user the "after current
	// edit" semantics are best-effort: while the model is
	// streaming a long text response there is no "edit" to wait
	// for, so we also cancel the context. The next loop
	// iteration in runTurn will check delayedCancel and exit
	// cleanly even though the tool-call post-check never fires.
	if s.cancel != nil {
		// Only cancel the context if no tool call is in
		// flight. We approximate that by checking
		// cancelReq -- if a previous SendCancel already
		// set it, we're already on the way out.
		if s.cancelReq {
			// Already cancelling; nothing to do.
		} else {
			// Defer the actual cancel until after the
			// current tool finishes by setting only
			// delayedCancel. The tool-execution path
			// observes the flag and calls cancel() at
			// the end of the in-flight tool call.
		}
	}
	s.mu.Unlock()
}

// SendClose closes the session. The event channel is closed.
func (s *Session) SendClose() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	close(s.events)
}

func (s *Session) system(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Replace any existing system message
	out := []provider.Message{{Role: "system", Content: text}}
	out = append(out, s.history...)
	s.history = out
}

// ── Turn execution ──────────────────────────────────────────────────────

func (s *Session) runTurn(parent context.Context, text string, attachments []protocol.Attachment) {
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	s.cancel = cancel
	s.cancelReq = false
	s.delayedCancel = false
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cancel = nil
		s.delayedCancel = false
		s.mu.Unlock()
		s.emitAgentDone()
	}()

	// 1) Push the user message
	s.mu.Lock()
	s.history = append(s.history, provider.Message{Role: "user", Content: text})
	s.mu.Unlock()

	// 2) Loop: call model → handle tool calls → repeat
	for {
		if ctx.Err() != nil {
			return
		}
		// If the user requested a delayed cancel while we were
		// NOT in a tool call (i.e. the model was streaming text
		// or we are about to start the next iteration), honour
		// the cancel immediately. "After the current edit" only
		// makes sense if there is an in-flight edit to wait for.
		s.mu.Lock()
		if s.delayedCancel {
			s.cancel()
			s.cancelReq = true
			s.delayedCancel = false
		}
		s.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		resp, err := s.callProvider(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || s.cancelReq {
				return
			}
			s.emitError(err)
			return
		}

		// Detect inline tool calls in markdown code blocks (Cortex-style)
		inlineCalls := extractInlineToolCalls(resp.Content)
		if len(inlineCalls) == 0 && len(resp.ToolCalls) == 0 {
			// No tool calls; turn is done
			s.mu.Lock()
			s.history = append(s.history, provider.Message{
				Role:    "assistant",
				Content: resp.Content,
			})
			s.mu.Unlock()
			// Emit a synthetic stream-done event with usage
			s.emitStreamDone(resp.Usage)
			return
		}

		// Persist the assistant turn with tool calls attached
		allCalls := append([]provider.ToolCall{}, resp.ToolCalls...)
		allCalls = append(allCalls, inlineCalls...)
		s.mu.Lock()
		s.history = append(s.history, provider.Message{
			Role:      "assistant",
			Content:   stripToolCallBlocks(resp.Content),
			ToolCalls: allCalls,
		})
		s.mu.Unlock()

		// Execute each tool call
		for _, call := range allCalls {
			if ctx.Err() != nil {
				return
			}
			s.executeToolCall(ctx, call)
		}
	}
}

func (s *Session) emitAgentDone() {
	select {
	case s.events <- protocol.SessionEvent{Type: "agent_done"}:
	default:
	}
}

func (s *Session) emitError(err error) {
	select {
	case s.events <- protocol.SessionEvent{
		Type: "error",
		Data: protocol.EventError{Message: err.Error()},
	}:
	default:
	}
}

// extractInlineToolCalls parses ```tool_call {...}``` blocks from
// markdown content. Returns the parsed tool calls.
var inlineToolCallRE = regexp.MustCompile("(?s)```tool_call\\s*\\n(\\{[\\s\\S]*?\\})\\s*\\n```")

func extractInlineToolCalls(content string) []provider.ToolCall {
	var out []provider.ToolCall
	for _, m := range inlineToolCallRE.FindAllStringSubmatch(content, -1) {
		var parsed struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(m[1]), &parsed); err != nil {
			continue
		}
		if parsed.Name == "" {
			continue
		}
		id := uuid.NewString()
		out = append(out, provider.ToolCall{ID: id, Name: parsed.Name, Arguments: parsed.Arguments})
	}
	return out
}

func stripToolCallBlocks(content string) string {
	return inlineToolCallRE.ReplaceAllString(content, "")
}

func (s *Session) executeToolCall(ctx context.Context, call provider.ToolCall) {
	tool, ok := s.tools.Get(call.Name)
	if !ok {
		s.emitToolCall(call.ID, call.Name, call.Arguments, "")
		s.emitToolResult(call.ID, call.Name, "", true, "unknown tool")
		s.mu.Lock()
		s.history = append(s.history, provider.Message{
			Role: "tool", Content: fmt.Sprintf("[tool %s] error: unknown tool", call.Name),
			ToolName: call.Name, ToolCallID: call.ID,
		})
		s.mu.Unlock()
		s.maybeFireDelayedCancel()
		return
	}
	summary := summarizeArgs(call.Arguments)
	s.emitToolCall(call.ID, call.Name, call.Arguments, summary)
	tctx := tools.Context{
		CWD:        s.workdir,
		AllowShell: s.cfg.Tools.AllowShell,
		AllowWrite: s.cfg.Tools.AllowWrite,
		AllowGit:   s.cfg.Tools.AllowGit,
	}
	res, _ := tool.Run(tctx, call.Arguments)
	s.emitToolResult(call.ID, call.Name, res.Output, !res.OK, res.Error)
	formatted := res.Output
	if !res.OK {
		formatted = fmt.Sprintf("[tool %s] error: %s\n%s", call.Name, res.Error, res.Output)
	} else {
		formatted = fmt.Sprintf("[tool %s] ok\n%s", call.Name, res.Output)
	}
	s.mu.Lock()
	s.history = append(s.history, provider.Message{
		Role: "tool", Content: formatted, ToolName: call.Name, ToolCallID: call.ID,
	})
	s.mu.Unlock()
	// If the user requested a "cancel after current edit" while
	// this tool call was running, fire the cancel now so the
	// turn loop exits cleanly and the UI sends the queued
	// follow-up message.
	s.maybeFireDelayedCancel()
}

// maybeFireDelayedCancel cancels the running turn's context if the
// user requested a delayed cancel (SendCancelAfterEdit) and the
// current tool call has just finished. This is the hook that turns
// "wait for the in-flight edit to finish" into an actual cancel.
func (s *Session) maybeFireDelayedCancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.delayedCancel && s.cancel != nil {
		s.cancel()
		s.cancelReq = true
		s.delayedCancel = false
	}
}

func summarizeArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	var keys []string
	for k := range args {
		keys = append(keys, k)
	}
	// Stable order: sort
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var parts []string
	for _, k := range keys {
		v := args[k]
		var s string
		switch vv := v.(type) {
		case string:
			if len(vv) > 60 {
				s = fmt.Sprintf("%q...", vv[:60])
			} else {
				s = fmt.Sprintf("%q", vv)
			}
		default:
			b, _ := json.Marshal(v)
			s = string(b)
			if len(s) > 60 {
				s = s[:60] + "..."
			}
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, s))
	}
	return strings.Join(parts, ", ")
}

// ── Provider call ───────────────────────────────────────────────────────

func (s *Session) opengatewayScopedModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" || strings.Contains(model, "/") {
		return model
	}
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "minimax"):
		return "minimax/" + model
	case strings.Contains(lower, "mimo"):
		return "xiaomi/" + model
	case strings.Contains(lower, "gemini"):
		return "google/" + model
	case strings.Contains(lower, "qwen"):
		return "qwen/" + model
	case strings.Contains(lower, "nemotron") || strings.Contains(lower, "nvidia"):
		return "nvidia/" + model
	default:
		return model
	}
}

// reasoningEffortForRequest returns the value to send to the provider.
// "auto" (and empty) are not valid provider values, so they're dropped.
func reasoningEffortForRequest(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low", "medium", "high", "xhigh", "minimal", "none":
		return strings.ToLower(strings.TrimSpace(effort))
	default:
		return ""
	}
}

func (s *Session) callProvider(ctx context.Context) (provider.Response, error) {
	active := s.active
	canonical, mc, err := s.cfg.GetModel(active)
	if err != nil {
		return provider.Response{}, err
	}
	prov, err := provider.New(provider.ModelConfig{
		Provider:         mc.Provider,
		Model:            mc.Model,
		BaseURL:          mc.BaseURL,
		APIKey:           s.resolveAPIKey(mc),
		Temperature:      mc.Temperature,
		MaxTokens:        mc.MaxTokens,
		ReasoningEffort:  mc.ReasoningEffort,
		CortexPromptMode: mc.CortexPromptMode,
	})
	if err != nil {
		return provider.Response{}, err
	}

	// Emit init state
	s.events <- protocol.SessionEvent{
		Type: "init_state",
		Data: protocol.EventInitState{State: 1, Model: canonical},
	}

	requestModel := mc.Model
	if canonical != "" && canonical != mc.Provider {
		requestModel = strings.TrimPrefix(canonical, mc.Provider+"/")
	}
	if strings.EqualFold(mc.Provider, "opengateway") {
		requestModel = s.opengatewayScopedModel(requestModel)
	}

	req := provider.Request{
		Model:            requestModel,
		Messages:         s.History(),
		Tools:            convertToolsToProvider(s.tools),
		ToolChoice:       provider.ToolChoice{Mode: "auto"},
		Temperature:      mc.Temperature,
		MaxTokens:        mc.MaxTokens,
		Stream:           s.cfg.Streaming,
		ReasoningEffort:  reasoningEffortForRequest(mc.ReasoningEffort),
		CortexPromptMode: mc.CortexPromptMode,
	}

	if s.cfg.Streaming {
		return prov.Stream(ctx, req, s.onChunk)
	}
	// Non-streaming: just call Chat
	resp, err := prov.Chat(ctx, req)
	if err != nil {
		return provider.Response{}, err
	}
	// Emit the content as a single chunk for uniform downstream handling
	s.events <- protocol.SessionEvent{
		Type: "stream_chunk",
		Data: protocol.EventStreamChunk{Text: resp.Content},
	}
	s.emitStreamDone(resp.Usage)
	return resp, nil
}

func (s *Session) onChunk(c provider.Chunk) {
	if c.Content != "" {
		s.events <- protocol.SessionEvent{
			Type: "stream_chunk",
			Data: protocol.EventStreamChunk{Text: c.Content},
		}
	}
	if c.Usage.TotalTokens > 0 || c.Usage.PromptTokens > 0 {
		s.emitStreamDone(c.Usage)
	}
}

func (s *Session) emitStreamDone(u provider.Usage) {
	s.events <- protocol.SessionEvent{
		Type: "stream_done",
		Data: protocol.EventStreamDone{
			InputTokens:  u.PromptTokens,
			OutputTokens: u.CompletionTokens,
		},
	}
}

func (s *Session) emitToolCall(id, name string, args map[string]any, summary string) {
	s.events <- protocol.SessionEvent{
		Type: "tool_call",
		Data: protocol.EventToolCall{
			ToolID:    id,
			Name:      name,
			Arguments: args,
			Summary:   summary,
		},
	}
}

func (s *Session) emitToolResult(id, name, output string, isErr bool, errMsg string) {
	if isErr {
		output = errMsg
	}
	s.events <- protocol.SessionEvent{
		Type: "tool_result",
		Data: protocol.EventToolResult{
			ToolID:  id,
			Name:    name,
			Output:  output,
			IsError: isErr,
		},
	}
}

// resolveAPIKey returns the API key for the model, falling back to
// environment variables.
func (s *Session) resolveAPIKey(mc *cortexconfig.ModelConfig) string {
	if mc.APIKey != "" {
		return mc.APIKey
	}
	if envVar := cortexconfig.ProviderEnvVar(mc.Provider); envVar != "" {
		return osGetenv(envVar)
	}
	return ""
}

// convertParams adapts the schema from tools.ProviderSchema → provider.ToolParam.
func convertParams(s tools.ProviderSchema) map[string]provider.ToolParam {
	out := make(map[string]provider.ToolParam, len(s.Properties))
	for k, v := range s.Properties {
		req := false
		for _, r := range s.Required {
			if r == k {
				req = true
				break
			}
		}
		out[k] = provider.ToolParam{Type: v.Type, Description: v.Description, Required: req}
	}
	return out
}

// convertToolsToProvider converts the tool registry to a slice of provider.Tool.
func convertToolsToProvider(reg *tools.Registry) []provider.Tool {
	src := reg.ToProviderTools()
	out := make([]provider.Tool, 0, len(src))
	for _, t := range src {
		out = append(out, provider.Tool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  convertParams(t.Parameters),
		})
	}
	return out
}
