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
	"sort"
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

	// userAnswerCh carries the user's response to a question
	// the model raised via the ask_user_question tool. The
	// tool handler blocks on this channel so the LLM sees
	// the real answer as the tool result — the previous
	// design returned a "pending" placeholder which left
	// the conversation in a bad state (the LLM had no
	// answer, the API rejected the next turn with a
	// "tool call and result not match" 400, and the user
	// couldn't send any more messages).
	userAnswerCh chan userAnswer

	// done is closed when the session is shutting down.
	// Blocking tool calls (e.g. ask_user_question) select
	// on it so they can exit promptly when the TUI quits
	// instead of leaving an orphan goroutine waiting
	// forever for a user answer.
	done chan struct{}

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

// userAnswer is the user's response to a question the model
// raised via the ask_user_question tool. `answer` is the
// label the user picked (or the free-text they typed for
// the "Type something." option); `text` is the
// supplementary text when the picked option had
// has_user_input=true; `batch` is a non-empty map when the
// answer is for a multi-question batch (id → answer).
type userAnswer struct {
	answer string
	text   string
	batch  map[string]string
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
		id:           uuid.NewString(),
		startedAt:    time.Now(),
		workdir:      cfg.Workdir,
		configDir:    cfg.ConfigDir,
		cfg:          cfg.CortexCfg,
		active:       cfg.ActiveModel,
		tools:        tools.NewRegistry(),
		events:       make(chan protocol.SessionEvent, 64),
		userAnswerCh: make(chan userAnswer, 1),
		done:         make(chan struct{}),
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
	systemMsg := DefaultSystemPrompt()
	if systemMsg != "" {
		systemMsg += "\n\n"
	}
	if s.cfg.SystemPrompt != "" {
		systemMsg += s.cfg.SystemPrompt + "\n\n"
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
	systemMsg := DefaultSystemPrompt()
	if systemMsg != "" {
		systemMsg += "\n\n"
	}
	if s.cfg.SystemPrompt != "" {
		systemMsg += s.cfg.SystemPrompt + "\n\n"
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

// SendUserAnswer feeds the user's response back to a
// handleAskUserQuestion call that is blocked on
// s.userAnswerCh. If no question is pending (the channel
// is full or the user pressed Esc on a closed panel) the
// answer is silently dropped — the session has nothing to
// do with it.
func (s *Session) SendUserAnswer(answer, text string) {
	if s.userAnswerCh == nil {
		return
	}
	select {
	case s.userAnswerCh <- userAnswer{answer: answer, text: text}:
	default:
		// Drop — there was no pending question (or one
		// was already answered). The TUI shouldn't
		// normally hit this path because the question
		// panel only emits an answer while it's open.
	}
}

// SendUserAnswerBatch feeds a multi-question response
// back to handleAskUserQuestion. Same drop semantics as
// SendUserAnswer.
func (s *Session) SendUserAnswerBatch(answers map[string]string) {
	if s.userAnswerCh == nil {
		return
	}
	select {
	case s.userAnswerCh <- userAnswer{batch: answers}:
	default:
	}
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
	// Close the done channel first so any goroutine
	// blocked in handleAskUserQuestion bails out before
	// we close s.events (which they might also be
	// selecting on).
	select {
	case <-s.done:
		// already closed
	default:
		close(s.done)
	}
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
			// and finish_reason. If finish_reason is
			// "length", the UI warns the user that
			// the model hit max_tokens instead of
			// silently looking like the agent
			// randomly stopped mid-sentence.
			s.emitStreamDone(resp.Usage, resp.FinishReason)
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

// safeEmit sends an event to s.events, dropping it if no
// one is listening (TUI is wedged) and recovering if the
// channel has been closed (SendClose was called while a
// goroutine was still in the middle of runTurn).
//
// The "send on closed channel" panic was the root cause of
// the user-reported "AI doesn't respond at all" symptom
// when they created a new session, sent a message, and then
// the TUI quit (or the session was torn down) before the
// goroutine finished. The goroutine would panic in its
// defer emitAgentDone, the Bubble Tea program would
// receive the panic and exit abruptly, and any subsequent
// messages routed to that session's client would silently
// fail because the underlying session was already shut
// down. Fix: every emit() helper now wraps the send in a
// defer/recover so a closed channel is treated as a
// drop-the-event condition, not a fatal panic.
func (s *Session) safeEmit(ev protocol.SessionEvent) {
	defer func() { _ = recover() }()
	select {
	case s.events <- ev:
	default:
	}
}

func (s *Session) emitAgentDone() {
	s.safeEmit(protocol.SessionEvent{Type: "agent_done"})
}

func (s *Session) emitError(err error) {
	s.safeEmit(protocol.SessionEvent{
		Type: "error",
		Data: protocol.EventError{Message: err.Error()},
	})
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
	// Special-case the structured UI tools (todo_write,
	// ask_user_question) BEFORE the generic Run() path. The
	// generic path still runs (so the result is recorded
	// and the model can see it in its history), but we also
	// emit a typed event so the TUI can update the right
	// panel / question overlay directly. Previously these
	// tools were registered as stubs that returned text, and
	// the user reported the AI 'fails to make a todo list
	// when asked' because nothing was actually rendering.
	switch call.Name {
	case "todo_write":
		s.handleTodoWrite(call)
		return
	case "ask_user_question":
		s.handleAskUserQuestion(call)
		return
	case "dispatch_workflow":
		s.handleDispatchWorkflow(call)
		return
	}
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
	// If the LLM produced a tool call whose JSON was
	// truncated (common for very large content the model
	// can't fit in its output token budget) the provider
	// layer falls back to storing the raw string in
	// `args["_raw"]`. The tool then sees an empty
	// `args["path"]` / `args["content"]` and rejects the
	// call with "path and content are required". That's
	// a dead end for the user — the agent has to retry
	// with smaller content. We try to recover by
	// scanning the raw string for the required fields
	// and stitching them back into the args map. If the
	// raw string is itself truncated (no closing quote)
	// we still get partial content; better than nothing.
	if raw, ok := call.Arguments["_raw"].(string); ok {
		call.Arguments = recoverArgsFromRaw(call.Name, call.Arguments, raw)
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

// handleTodoWrite parses a todo_write tool call and emits a
// structured EventTodoListUpdated. The TUI listens for that
// event and renders the todo list in the right panel. Without
// this hook the AI's todo list never reached the UI and the
// user reported 'the AI never makes a todo list when asked'.
func (s *Session) handleTodoWrite(call provider.ToolCall) {
	summary := summarizeArgs(call.Arguments)
	s.emitToolCall(call.ID, call.Name, call.Arguments, summary)

	var parsed []map[string]any
	switch v := call.Arguments["todos"].(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			s.emitToolResult(call.ID, call.Name, "", true, "todos: invalid JSON: "+err.Error())
			s.recordToolResult(call.ID, call.Name, "", true, "todos: invalid JSON: "+err.Error())
			return
		}
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				parsed = append(parsed, m)
			}
		}
	default:
		s.emitToolResult(call.ID, call.Name, "", true, "todos: unsupported type")
		s.recordToolResult(call.ID, call.Name, "", true, "todos: unsupported type")
		return
	}
	if len(parsed) == 0 {
		s.emitToolResult(call.ID, call.Name, "", true, "todos: empty list")
		s.recordToolResult(call.ID, call.Name, "", true, "todos: empty list")
		return
	}

	items := make([]protocol.TodoItem, 0, len(parsed))
	for i, p := range parsed {
		content, _ := p["content"].(string)
		if content == "" {
			content, _ = p["activeForm"].(string)
		}
		status, _ := p["status"].(string)
		id, _ := p["id"].(string)
		if id == "" {
			id = fmt.Sprintf("todo-%d", i+1)
		}
		items = append(items, protocol.TodoItem{
			ID:      id,
			Content: content,
			Status:  protocol.TodoStatus(status),
		})
	}
	s.safeEmit(protocol.SessionEvent{
		Type: "todo_list_updated",
		Data: protocol.EventTodoListUpdated{Todos: items},
	})
	out := fmt.Sprintf("updated %d todo(s)", len(items))
	s.emitToolResult(call.ID, call.Name, out, false, "")
	s.recordToolResult(call.ID, call.Name, out, false, "")
}

// handleAskUserQuestion parses an ask_user_question tool
// call and emits a structured EventUserQuestion. The TUI
// opens the question panel overlay so the user can pick
// one of the options interactively. We then BLOCK on
// s.userAnswerCh until the user submits an answer (or
// cancels), and the user's response becomes the tool
// result that the LLM sees in its history. This is the
// correct shape of the conversation: the LLM asked a
// question, the user answered it, the model can now act
// on the answer.
//
// The previous version emitted the event and returned a
// "pending" placeholder string. That broke the API
// protocol — the LLM had no real answer, so the next turn
// was rejected with HTTP 400 "tool call and result not
// match" (providers like MiniMax validate the tool-call
// ↔ tool-result pairing) and the user couldn't send any
// more messages.
//
// We accept the `options` argument in three formats,
// because the LLM doesn't always serialise it the same
// way:
//
//   - string:  JSON-encoded array of {label, description} objects
//   - []any:   already-parsed array of {label, description} objects
//   - map[string]any: Anthropic-style {"item": [...]}
//     wrapper, or {"options": [...]}, or {"questions": [...]}
//
// In all three cases we end up with a normalised
// []protocol.EventQuestionOption that the question panel
// can render.
func (s *Session) handleAskUserQuestion(call provider.ToolCall) {
	summary := summarizeArgs(call.Arguments)
	s.emitToolCall(call.ID, call.Name, call.Arguments, summary)

	question, _ := call.Arguments["question"].(string)
	if question == "" {
		s.emitToolResult(call.ID, call.Name, "", true, "question is required")
		s.recordToolResult(call.ID, call.Name, "", true, "question is required")
		return
	}
	opts := parseAskUserQuestionOptions(call.Arguments["options"])
	if len(opts) < 2 {
		s.emitToolResult(call.ID, call.Name, "", true, "options: need at least 2 options")
		s.recordToolResult(call.ID, call.Name, "", true, "options: need at least 2 options")
		return
	}
	if len(opts) > 4 {
		opts = opts[:4]
	}
	header, _ := call.Arguments["header"].(string)
	if header == "" {
		header = "Question"
	}

	// Emit the question event so the TUI pops the panel.
	// This is a blocking send — the events channel is
	// buffered to 64, but if the TUI is wedged (or has
	// closed) we'd block forever. Guard with a short
	// timeout via select+default, and recover from
	// "send on closed channel" panics (via safeEmit).
	sent := false
	defer func() { _ = recover() }()
	select {
	case s.events <- protocol.SessionEvent{
		Type: "user_question",
		Data: protocol.EventUserQuestion{
			Question:    question,
			Options:     nil,
			RichOptions: opts,
			Category:    header,
		},
	}:
		sent = true
	default:
		// TUI not listening — degrade to a synchronous
		// answer of "skip" so the LLM can keep going.
		out := "skipped (TUI unavailable)"
		s.emitToolResult(call.ID, call.Name, out, false, "")
		s.recordToolResult(call.ID, call.Name, out, false, "")
		return
	}
	_ = sent

	// Block until the user answers, cancels, or the
	// session is closed. The TUI pushes exactly one of:
	//   - SendUserAnswer(answer, text) for a single answer
	//   - SendUserAnswerBatch(map) for a multi-question
	//     batch (the simple ask_user_question path never
	//     uses this; it's here so any future call to the
	//     tool that wants multiple questions works)
	//   - SendClose (when the user quits the TUI) which
	//     closes s.done
	var ua userAnswer
	select {
	case ua = <-s.userAnswerCh:
	case <-s.done:
		// Session was closed while we were waiting.
		out := "skipped (session closed)"
		s.emitToolResult(call.ID, call.Name, out, false, "")
		s.recordToolResult(call.ID, call.Name, out, false, "")
		return
	}

	// Build the tool result from the user's response.
	// The LLM sees this in its history on the next turn
	// and can act on it directly.
	var result string
	switch {
	case ua.batch != nil:
		// Multi-question batch: serialise the answers
		// map so the model gets a structured response.
		// The format is <id>=<answer> per line, which
		// reads naturally in chat.
		var b strings.Builder
		// Stable order so the model sees the same
		// string regardless of Go map iteration.
		ids := make([]string, 0, len(ua.batch))
		for id := range ua.batch {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			fmt.Fprintf(&b, "%s=%s\n", id, ua.batch[id])
		}
		result = strings.TrimRight(b.String(), "\n")
	case ua.answer == "":
		// User pressed Esc — tell the LLM they skipped.
		result = "(user dismissed the question without answering)"
	default:
		if ua.text != "" {
			result = ua.answer + ": " + ua.text
		} else {
			result = ua.answer
		}
	}
	s.emitToolResult(call.ID, call.Name, result, false, "")
	// Record the tool result in history so the LLM
	// sees the answer on the next turn. The generic
	// executeToolCall path does this itself, but the
	// special-case handlers return early and need to
	// do it explicitly. Without this, the LLM's prior
	// tool-call has no matching tool-result in its
	// history, and providers like MiniMax reject the
	// next request with HTTP 400 "tool call and
	// result not match".
	s.recordToolResult(call.ID, call.Name, result, false, "")
}

// handleDispatchWorkflow is the session-side handler for
// the dispatch_workflow tool. The user reported: "the
// agent isnt using workflows, it might nto be in its
// system prompt at all, itj ust starts working by itself
// it doesnt seem to know". Before this tool existed,
// the system prompt told the agent to "suggest /workflow
// <prompt>" — but the agent had no way to actually
// dispatch the workflow itself, so it just printed the
// suggestion and proceeded to do the work alone. The
// fix: register dispatch_workflow as a real tool. The
// tool's Run() returns a marker; the session handler
// intercepts the call, emits an EventWorkflowDispatch
// (which the TUI uses to actually start the workflow),
// and records a tool result so the LLM sees a
// successful call.
//
// We do NOT block the LLM here — the workflow runs in
// the background and reports back via the normal
// workflow event channel. The LLM can move on, and the
// orchestrator will inject a summary message when it
// finishes.
func (s *Session) handleDispatchWorkflow(call provider.ToolCall) {
	prompt, _ := call.Arguments["prompt"].(string)
	if prompt == "" {
		s.emitToolCall(call.ID, call.Name, call.Arguments, "")
		s.emitToolResult(call.ID, call.Name, "", true, "prompt is required")
		s.recordToolResult(call.ID, call.Name, "error: prompt is required", true, "prompt is required")
		return
	}
	preset, _ := call.Arguments["preset"].(string)

	// Emit the typed event so the TUI can start the
	// workflow. The handler in internal/ui/model.go
	// picks this up and runs the same code path as
	// the /workflow slash command.
	s.emitToolCall(call.ID, call.Name, call.Arguments, prompt)
	s.safeEmit(protocol.SessionEvent{
		Type: "workflow_dispatch",
		Data: protocol.EventWorkflowDispatch{
			Prompt: prompt,
			Preset: preset,
		},
	})

	// Confirm to the LLM that the call succeeded.
	// The LLM will see this and know the workflow
	// is running in the background. The
	// orchestrator will deliver a summary message
	// when the workflow finishes.
	confirm := fmt.Sprintf("workflow dispatched with prompt: %q", prompt)
	if preset != "" {
		confirm = fmt.Sprintf("workflow dispatched with preset=%s, prompt: %q", preset, prompt)
	}
	confirm += "\n\nThe orchestrator will plan, implement, review, and test the work. A summary will arrive as a normal chat message when it finishes — keep chatting with the user in the meantime if they have questions."
	s.emitToolResult(call.ID, call.Name, confirm, false, "")
	s.recordToolResult(call.ID, call.Name, confirm, false, "")
}

// parseAskUserQuestionOptions normalises the `options`
// argument into []protocol.EventQuestionOption. See the
// doc-comment on handleAskUserQuestion for the accepted
// shapes.
func parseAskUserQuestionOptions(raw any) []protocol.EventQuestionOption {
	if raw == nil {
		return nil
	}
	// Case 1: JSON-encoded string.
	if s, ok := raw.(string); ok {
		var arr []struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal([]byte(s), &arr); err == nil {
			return toEventOptions(arr)
		}
		// Could also be a single object stringified;
		// try wrapping it in an array.
		var single struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal([]byte(s), &single); err == nil && single.Label != "" {
			return toEventOptions([]struct {
				Label       string `json:"label"`
				Description string `json:"description"`
			}{single})
		}
		return nil
	}
	// Case 2: already-parsed array of objects.
	if arr, ok := raw.([]any); ok {
		var out []struct {
			Label       string `json:"label"`
			Description string `json:"description"`
		}
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				var row struct {
					Label       string `json:"label"`
					Description string `json:"description"`
				}
				if l, ok := m["label"].(string); ok {
					row.Label = l
				} else if l, ok := m["title"].(string); ok {
					row.Label = l
				}
				if d, ok := m["description"].(string); ok {
					row.Description = d
				}
				out = append(out, row)
			}
		}
		return toEventOptions(out)
	}
	// Case 3: object wrapping an array. Try the common
	// keys in turn.
	if m, ok := raw.(map[string]any); ok {
		for _, key := range []string{"item", "items", "options", "questions"} {
			if v, ok := m[key]; ok {
				if opts := parseAskUserQuestionOptions(v); len(opts) > 0 {
					return opts
				}
			}
		}
	}
	return nil
}

func toEventOptions(arr []struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}) []protocol.EventQuestionOption {
	out := make([]protocol.EventQuestionOption, 0, len(arr))
	for _, o := range arr {
		if o.Label == "" {
			continue
		}
		out = append(out, protocol.EventQuestionOption{
			Title:       o.Label,
			Description: o.Description,
		})
	}
	return out
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

// recoverArgsFromRaw tries to recover the required
// arguments from a tool call whose JSON was truncated
// by the provider's output token limit. The provider
// layer (internal/provider/openai_compat.go) stores the
// raw partial JSON in `args["_raw"]` when it can't parse
// it. We scan the raw string for the field names the
// tool needs and extract their string values.
//
// The user reported: "shell commands are also being
// truncated when content is too large" and "ERROR:
// command is required" / "ERROR: path and content are
// required". The root cause is the LLM producing a
// 50KB+ write_file call where the JSON arguments
// exceed the output token budget, so the SSE stream
// cuts off mid-string. The raw partial string is still
// 99% complete; we just have to find the field
// boundaries.
//
// This is a best-effort recovery. We can't recover
// cleanly if a string is missing its closing quote (the
// raw content was genuinely truncated), but for the
// "truncated trailing fields" case (the JSON object's
// closing brace was cut off) we recover everything up
// to the truncation point.
func recoverArgsFromRaw(toolName string, args map[string]any, raw string) map[string]any {
	if raw == "" {
		return args
	}
	// Field list per tool. The agent can re-run the
	// tool with the recovered content (or, in the
	// case of write_file, the recovered content may
	// be incomplete and the agent has to retry with
	// the rest in a follow-up call).
	var fields []string
	switch toolName {
	case "write_file":
		fields = []string{"path", "content"}
	case "edit_file":
		fields = []string{"path", "oldString", "newString"}
	case "read_file":
		fields = []string{"path"}
	case "delete_file":
		fields = []string{"path"}
	case "run_shell":
		fields = []string{"command"}
	}
	if len(fields) == 0 {
		return args
	}
	// If the field for this tool is already populated
	// (the JSON partial-parsed successfully), no work
	// to do. (We only enter this function when the
	// provider fell back to `_raw`, but a tool with no
	// required string fields would land here as a
	// no-op.)
	alreadyComplete := true
	for _, field := range fields {
		if v, ok := args[field].(string); !ok || v == "" {
			alreadyComplete = false
			break
		}
	}
	if alreadyComplete {
		return args
	}
	for _, field := range fields {
		// Skip if the field is already populated
		// from a successful partial parse.
		if v, ok := args[field].(string); ok && v != "" {
			continue
		}
		// Find "field": "... and walk to the end of
		// the string. We do NOT require a closing
		// quote: a truncated JSON has the field
		// value as the last thing in the stream.
		needle := "\"" + field + "\":"
		idx := strings.Index(raw, needle)
		if idx < 0 {
			continue
		}
		// Skip past the colon, then any whitespace.
		idx += len(needle)
		for idx < len(raw) && (raw[idx] == ' ' || raw[idx] == '\t' || raw[idx] == '\n') {
			idx++
		}
		if idx >= len(raw) || raw[idx] != '"' {
			continue
		}
		idx++ // skip opening quote
		// Walk to the end of the string. We allow
		// the closing quote to be missing (truncated
		// JSON) — in that case we take everything
		// from `idx` to EOF.
		end := idx
		for end < len(raw) {
			if raw[end] == '\\' && end+1 < len(raw) {
				// Skip escape sequence.
				end += 2
				continue
			}
			if raw[end] == '"' {
				break
			}
			end++
		}
		// If we found a closing quote, exclude it.
		value := raw[idx:end]
		// Unescape the most common JSON escapes. We
		// don't need to handle every edge case —
		// the agent will re-run if the result is
		// slightly off.
		value = strings.ReplaceAll(value, `\"`, `"`)
		value = strings.ReplaceAll(value, `\\`, `\`)
		value = strings.ReplaceAll(value, `\n`, "\n")
		value = strings.ReplaceAll(value, `\t`, "\t")
		args[field] = value
	}
	return args
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
	s.safeEmit(protocol.SessionEvent{
		Type: "init_state",
		Data: protocol.EventInitState{State: 1, Model: canonical},
	})

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

	// Fix orphan tool results: some strict
	// providers (MiniMax, certain OpenRouter
	// backends) reject requests with HTTP 400
	// 'tool result's tool id() not found' when
	// a `role: tool` message references a
	// tool_call_id that the previous assistant
	// message didn't include in its tool_calls
	// array. This can happen when:
	//   - The session was restored from disk
	//     and the assistant's tool_calls were
	//     dropped (we only persist the rendered
	//     chat text) but the tool results were
	//     kept
	//   - The LLM emitted an inline `tool_call`
	//     markdown block AND the provider also
	//     returned its own tool_calls, and the
	//     we recorded results for the inline ones
	//     that the provider didn't recognise
	//   - A tool result for one turn leaks into
	//     a later turn (provider error handling)
	//
	// Rather than fail the request, we drop any
	// orphan tool result messages from the
	// outgoing history. The model still has the
	// text context from the chat history, so it
	// can continue the conversation; it just
	// won't see a tool result for a tool call
	// it didn't see.
	req.Messages = stripOrphanToolResults(req.Messages)
	if s.cfg.Streaming {
		return prov.Stream(ctx, req, s.onChunk)
	}
	// Non-streaming: just call Chat
	resp, err := prov.Chat(ctx, req)
	if err != nil {
		return provider.Response{}, err
	}
	// Emit the content as a single chunk for uniform downstream handling
	s.safeEmit(protocol.SessionEvent{
		Type: "stream_chunk",
		Data: protocol.EventStreamChunk{Text: resp.Content},
	})
	s.emitStreamDone(resp.Usage, resp.FinishReason)
	return resp, nil
}

func (s *Session) onChunk(c provider.Chunk) {
	if c.Content != "" {
		s.safeEmit(protocol.SessionEvent{
			Type: "stream_chunk",
			Data: protocol.EventStreamChunk{Text: c.Content},
		})
	}
	if c.Usage.TotalTokens > 0 || c.Usage.PromptTokens > 0 || c.FinishReason != "" {
		s.emitStreamDone(c.Usage, c.FinishReason)
	}
}

func (s *Session) emitStreamDone(u provider.Usage, finish provider.FinishReason) {
	s.safeEmit(protocol.SessionEvent{
		Type: "stream_done",
		Data: protocol.EventStreamDone{
			InputTokens:  u.PromptTokens,
			OutputTokens: u.CompletionTokens,
			FinishReason: string(finish),
		},
	})
}

func (s *Session) emitToolCall(id, name string, args map[string]any, summary string) {
	s.safeEmit(protocol.SessionEvent{
		Type: "tool_call",
		Data: protocol.EventToolCall{
			ToolID:    id,
			Name:      name,
			Arguments: args,
			Summary:   summary,
		},
	})
}

func (s *Session) emitToolResult(id, name, output string, isErr bool, errMsg string) {
	if isErr {
		output = errMsg
	}
	s.safeEmit(protocol.SessionEvent{
		Type: "tool_result",
		Data: protocol.EventToolResult{
			ToolID:  id,
			Name:    name,
			Output:  output,
			IsError: isErr,
		},
	})
}

// recordToolResult appends a tool message to the
// conversation history so the LLM sees the result on the
// next turn. The generic executeToolCall path does this
// itself, but the special-case handlers (handleTodoWrite,
// handleAskUserQuestion) return early and need to record
// the result themselves. Without this, the LLM's
// tool-call has no matching tool-result in history and
// providers like MiniMax reject the next request with
// HTTP 400 "tool call and result not match".
func (s *Session) recordToolResult(id, name, output string, isErr bool, errMsg string) {
	if isErr {
		output = errMsg
	}
	content := fmt.Sprintf("[tool %s] %s", name, output)
	if isErr {
		content = fmt.Sprintf("[tool %s] error: %s", name, output)
	}
	s.mu.Lock()
	s.history = append(s.history, provider.Message{
		Role:       "tool",
		Content:    content,
		ToolName:   name,
		ToolCallID: id,
	})
	s.mu.Unlock()
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

// stripOrphanToolResults walks the outgoing
// conversation history and drops any `role: tool`
// message whose tool_call_id doesn't appear in the
// previous assistant message's tool_calls array.
// Strict providers (MiniMax, certain OpenRouter
// backends) reject the request with HTTP 400
// "tool result's tool id() not found" otherwise.
// The fix is defensive: we don't trust the history
// to be perfectly consistent (session restore,
// inline-vs-provider tool call duplication, etc.
// can leave dangling tool results), and we'd
// rather drop a result than fail the whole turn.
func stripOrphanToolResults(msgs []provider.Message) []provider.Message {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]provider.Message, 0, len(msgs))
	// Set of valid tool_call_ids from the most
	// recent assistant message that has
	// tool_calls. A `role: tool` message is only
	// valid if its tool_call_id is in this set.
	validIDs := map[string]bool{}
	for _, m := range msgs {
		switch m.Role {
		case "assistant":
			// Reset: a new assistant message
			// means previous tool_call_ids are
			// no longer valid for following
			// messages. Each assistant turn
			// owns its own tool_call_ids.
			validIDs = map[string]bool{}
			for _, tc := range m.ToolCalls {
				if tc.ID != "" {
					validIDs[tc.ID] = true
				}
			}
			out = append(out, m)
		case "tool":
			if validIDs[m.ToolCallID] {
				out = append(out, m)
			}
			// else: orphan — drop silently.
		default:
			// system / user messages: pass
			// through, they don't reference
			// tool_call_ids.
			out = append(out, m)
		}
	}
	return out
}
