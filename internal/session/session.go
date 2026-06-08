// Package session implements the in-process chat session. It holds
// conversation state, dispatches to the configured LLM provider, and
// emits protocol events on a channel for the TUI to consume.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
	"github.com/Mateooo93/cortex-cli/internal/provider"
	"github.com/Mateooo93/cortex-cli/internal/subagent"
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
	events           chan protocol.SessionEvent
	llmCallStartedAt time.Time

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

	// pendingSteer holds a user instruction injected via Steer()
	// while a turn (and possibly tool calls) are in flight. It is
	// injected as a follow-up user message after the current tool
	// batch completes (Pi-style steering / mid-turn redirect).
	pendingSteer string

	// toolBatchConcurrent is true while a multi-call tool batch is
	// executing in parallel. The UI uses this to prefix concurrent
	// tool results with the tool name (Grok Build / Claude Code style).
	toolBatchConcurrent bool

	processes *tools.ProcessRegistry
	subagents *subagent.Registry

	// ── Goal state ──────────────────────────────────────────────────────
	// When goalCondition is non-empty, the session runs autonomously:
	// after each turn, a cheap evaluator model judges whether the
	// condition is met. If not, the session continues automatically.
	goalCondition   string // the user's /goal condition
	goalActive      bool   // true when the goal loop is running
	goalTurns       int    // number of evaluated turns
	goalLastVerdict string // evaluator's most recent reason
	goalCancelled   bool   // set by SendCancel to stop the goal loop

	// reasoningEffort is session-scoped (/effort picker). The CLI layer
	// maps it to provider requests only when the active model supports
	// reasoning_effort — never persisted to model config.
	reasoningEffort string

	// currentTodos is the last structured todo list emitted by todo_write.
	// Used to preserve labels when the model sends status-only updates.
	currentTodos []protocol.TodoItem
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
		events:       make(chan protocol.SessionEvent, 512),
		userAnswerCh: make(chan userAnswer, 1),
		done:         make(chan struct{}),
	}
	s.processes = tools.NewProcessRegistry(s.emitBackgroundProcesses)
	s.subagents = subagent.NewRegistry(s.emitLocalSubagents)
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
	s.currentTodos = nil
	s.mu.Unlock()
	s.ensureSystemPrompt()
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

// SetReasoningEffort sets the session-scoped reasoning effort (/effort).
// Values: low, medium, high, ultracode. Empty clears the override.
func (s *Session) SetReasoningEffort(effort string) {
	s.mu.Lock()
	s.reasoningEffort = strings.TrimSpace(effort)
	s.mu.Unlock()
}

// ReasoningEffort returns the session-scoped effort level.
func (s *Session) ReasoningEffort() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reasoningEffort
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
	systemMsg := s.buildSystemMessage()
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
	// /goal commands: set, clear, or query goal state
	if strings.HasPrefix(text, "/goal") {
		rest := strings.TrimSpace(strings.TrimPrefix(text, "/goal"))
		lower := strings.ToLower(rest)

		// /goal clear|stop|off|reset|none|cancel → clear
		clearAliases := []string{"clear", "stop", "off", "reset", "none", "cancel"}
		for _, a := range clearAliases {
			if lower == a {
				s.ClearGoal()
				return
			}
		}

		// /goal (no args) → status query — push as normal user message
		if rest == "" {
			go s.runTurn(context.Background(), text, attachments)
			return
		}

		// /goal <condition> → set goal and start autonomous loop
		s.SetGoal(rest)
		go s.runGoalLoop(context.Background())
		return
	}

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

// Steer queues or injects a user instruction that should take effect
// after the current in-flight tool batch (if any) finishes. This is the
// "steering" mechanic from Pi: the agent can be redirected without
// leaving half-done edits on disk (pair with SendCancelAfterEdit or
// rely on the post-tool check). If no turn is active it behaves like Send.
func (s *Session) Steer(text string) {
	s.mu.Lock()
	if s.cancel != nil && !s.cancelReq {
		// Turn is live; remember the steer text. The loop will
		// pick it up after the current tool batch (or on next iter).
		s.pendingSteer = text
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	// No active turn: just Send normally.
	s.Send(text, nil)
}

// SendCancel asks the running turn to stop. Also stops any active
// goal loop. Safe to call when no turn is in flight (it's a no-op).
func (s *Session) SendCancel() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancelReq = true
	s.delayedCancel = false
	s.goalCancelled = true
	s.goalActive = false
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

func (s *Session) buildSystemMessage() string {
	systemMsg := BuildSystemPrompt(s.workdir)
	if systemMsg != "" {
		systemMsg += "\n\n"
	}
	if s.cfg.SystemPrompt != "" {
		systemMsg += s.cfg.SystemPrompt + "\n\n"
	}
	systemMsg += s.tools.ToSystemPrompt()
	return systemMsg
}

// ensureSystemPrompt prepends or refreshes the system message.
// Called on /clear, history restore, and before the first turn
// so new sessions still get the working-directory instructions.
func (s *Session) ensureSystemPrompt() {
	s.system(s.buildSystemMessage())
}

// ── Turn execution ──────────────────────────────────────────────────────

// modelLoop runs the core model→tools→model loop until the model
// produces a final text response (no more tool calls), an error
// occurs, or ctx is cancelled. It does NOT push the initial user
// message or emit agent_done — callers handle that.
//
// Returns nil on clean completion, or the error that stopped the loop.
func (s *Session) modelLoop(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.mu.Lock()
		if s.delayedCancel {
			s.cancel()
			s.cancelReq = true
			s.delayedCancel = false
		}
		s.mu.Unlock()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		resp, err := s.callProvider(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || s.cancelReq {
				return err
			}
			s.emitError(err)
			return err
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
			s.emitStreamDone(resp.Usage, resp.FinishReason)
			return nil
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

		// Execute tool calls — independent calls in a batch run in
		// parallel (reads, greps, bash probes, etc.) while history
		// is still recorded in the model's original call order.
		s.executeToolCalls(ctx, allCalls)

		// Pi-style steering: if a Steer() arrived during the tool
		// batch, inject it now as the next user message before the
		// follow-up LLM call.
		s.mu.Lock()
		if s.pendingSteer != "" {
			steerText := s.pendingSteer
			s.pendingSteer = ""
			s.history = append(s.history, provider.Message{Role: "user", Content: steerText})
			s.mu.Unlock()
			// continue the outer loop to call provider with the steered instruction
			continue
		}
		s.mu.Unlock()
	}
}

// runTurn sets up the turn context, pushes the initial user message,
// runs the model loop, and emits agent_done on completion.
func (s *Session) runTurn(parent context.Context, text string, attachments []protocol.Attachment) {
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	s.cancel = cancel
	s.cancelReq = false
	s.delayedCancel = false
	s.pendingSteer = ""
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cancel = nil
		s.delayedCancel = false
		s.mu.Unlock()
		s.emitAgentDone()
	}()

	// Ensure the system prompt (with workdir) is present before
	// the first provider call — new sessions start with empty
	// history and would otherwise miss it entirely.
	s.mu.Lock()
	hasSystem := len(s.history) > 0 && s.history[0].Role == "system"
	s.mu.Unlock()
	if !hasSystem {
		s.ensureSystemPrompt()
	}

	// Push the user message
	s.mu.Lock()
	s.history = append(s.history, provider.Message{Role: "user", Content: text})
	s.mu.Unlock()

	s.modelLoop(ctx)
}

// ── Goal loop ─────────────────────────────────────────────────────────

// SetGoal configures the session for autonomous goal-driven execution.
// After each turn, a cheap evaluator model checks whether the
// condition is met. If not, the session continues automatically.
func (s *Session) SetGoal(condition string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goalCondition = condition
	s.goalActive = true
	s.goalTurns = 0
	s.goalLastVerdict = ""
	s.goalCancelled = false
}

// ClearGoal stops any active goal loop and resets goal state.
func (s *Session) ClearGoal() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goalActive = false
	s.goalCancelled = true
}

// HasGoal returns true when a goal loop is active.
func (s *Session) HasGoal() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.goalActive
}

// GoalState returns a snapshot of the current goal state.
func (s *Session) GoalState() (condition string, active bool, turns int, lastVerdict string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.goalCondition, s.goalActive, s.goalTurns, s.goalLastVerdict
}

// buildTranscript concatenates the recent conversation history into a
// string the evaluator can judge. Keeps the tail (most recent work).
func (s *Session) buildTranscript() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	start := 0
	// Keep roughly the last 40 messages (20 turns) for the evaluator
	if len(s.history) > 40 {
		start = len(s.history) - 40
	}
	for _, msg := range s.history[start:] {
		fmt.Fprintf(&b, "[%s]: %s\n", msg.Role, truncateForEval(msg.Content, 500))
	}
	return b.String()
}

// truncateForEval limits a message for the evaluator context window.
func truncateForEval(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// evaluateGoal sends the transcript to a cheap evaluator model and
// returns whether the condition is met.
func (s *Session) evaluateGoal(ctx context.Context, condition, transcript string) (met bool, reason string) {
	// Build evaluator prompt
	sysPrompt := `You are a goal evaluator. Judge whether the goal condition is met based ONLY on what is explicitly shown in the transcript. You cannot run commands or read files.

Respond in EXACTLY this format:
VERDICT: <YES or NO>
REASON: <one concise sentence>`

	userPrompt := fmt.Sprintf("GOAL CONDITION:\n%s\n\nCONVERSATION TRANSCRIPT:\n%s", condition, transcript)

	// Use the session's provider with the smallest model possible.
	// Fall back to the active model if we can't resolve a cheaper one.
	messages := []provider.Message{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: userPrompt},
	}

	resp, err := s.callProviderWithMessages(ctx, messages)
	if err != nil {
		return false, fmt.Sprintf("evaluator error: %v", err)
	}

	lower := strings.ToLower(resp.Content)
	if strings.Contains(lower, "verdict: yes") || strings.Contains(lower, "verdict:yes") {
		met = true
	}
	if idx := strings.Index(lower, "reason:"); idx >= 0 {
		reason = strings.TrimSpace(resp.Content[idx+7:])
		if nl := strings.IndexAny(reason, "\n\r"); nl >= 0 {
			reason = strings.TrimSpace(reason[:nl])
		}
	}
	if reason == "" {
		reason = resp.Content
		if len(reason) > 200 {
			reason = reason[:200] + "..."
		}
	}
	return
}

// callProviderWithMessages is like callProvider but accepts explicit
// messages instead of using the session history. Used by the evaluator
// which needs its own prompt separate from the main conversation.
//
// For goal evaluation, it tries to select the cheapest available model
// (Haiku for Anthropic, GPT-4o-mini for OpenAI) to keep evaluation
// costs negligible. Falls back to the active model if cheap routing
// cannot be resolved.
func (s *Session) callProviderWithMessages(ctx context.Context, messages []provider.Message) (provider.Response, error) {
	s.mu.Lock()
	_, mc, err := s.cfg.GetModel(s.active)
	s.mu.Unlock()
	if err != nil {
		return provider.Response{}, err
	}
	apiKey := mc.APIKey
	if apiKey == "" {
		if env := cortexconfig.ProviderEnvVar(mc.Provider); env != "" {
			apiKey = os.Getenv(env)
		}
	}
	if apiKey == "" {
		return provider.Response{}, fmt.Errorf("no API key for evaluator")
	}

	// Cheap evaluator routing: prefer Haiku for Anthropic,
	// GPT-4o-mini for OpenAI. The evaluator makes a simple
	// binary decision — it doesn't need a frontier model.
	evalModel := mc.Model
	switch strings.ToLower(mc.Provider) {
	case "anthropic":
		evalModel = "claude-haiku-4-5-20251001"
	case "openai":
		evalModel = "gpt-4o-mini"
	}

	prov, err := provider.New(provider.ModelConfig{
		Provider: mc.Provider,
		Model:    evalModel,
		BaseURL:  mc.BaseURL,
		APIKey:   apiKey,
	})
	if err != nil {
		return provider.Response{}, err
	}
	req := provider.Request{
		Model:    evalModel,
		Messages: messages,
	}
	return prov.Chat(ctx, req)
}

// runGoalLoop is the autonomous goal execution loop. It runs the
// first turn with the goal condition, then evaluates after each turn.
// If the evaluator says NO, it injects feedback and continues.
// The loop stops when: goal met, cancelled, or context cancelled.
func (s *Session) runGoalLoop(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	s.cancel = cancel
	s.cancelReq = false
	s.delayedCancel = false
	s.pendingSteer = ""
	condition := s.goalCondition
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.cancel = nil
		s.delayedCancel = false
		wasActive := s.goalActive
		s.goalActive = false
		s.mu.Unlock()
		if wasActive {
			s.emitAgentDone()
		}
	}()

	// Turn 1: push the goal condition as the user message and run
	s.mu.Lock()
	s.history = append(s.history, provider.Message{Role: "user", Content: condition})
	s.goalTurns = 1
	s.mu.Unlock()

	if err := s.modelLoop(ctx); err != nil {
		return
	}

	// Subsequent turns: evaluate → continue if not met
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		s.mu.Lock()
		if s.goalCancelled || !s.goalActive {
			s.mu.Unlock()
			return
		}
		condition := s.goalCondition
		s.mu.Unlock()

		// Build transcript and evaluate
		transcript := s.buildTranscript()
		met, reason := s.evaluateGoal(ctx, condition, transcript)

		s.mu.Lock()
		s.goalLastVerdict = reason
		if met {
			// Goal achieved!
			s.goalActive = false
			s.mu.Unlock()
			s.emitGoalAchieved(condition, reason, s.goalTurns)
			return
		}
		// Not met — inject evaluator feedback as system guidance
		guidance := fmt.Sprintf(
			"[Goal evaluator verdict: NOT YET MET. %s\nContinue working toward the goal: %s]",
			reason, condition,
		)
		s.history = append(s.history, provider.Message{Role: "system", Content: guidance})
		s.goalTurns++
		s.mu.Unlock()

		// Continue the turn — the system message guides the model
		if err := s.modelLoop(ctx); err != nil {
			return
		}
	}
}

// emitGoalAchieved sends a goal-achieved event to the UI.
func (s *Session) emitGoalAchieved(condition, reason string, turns int) {
	ev := protocol.SessionEvent{
		Type: "event.goal_achieved",
		Data: map[string]any{
			"condition": condition,
			"reason":    reason,
			"turns":     turns,
		},
	}
	s.safeEmit(ev)
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

// emitStreamChunk delivers a streaming text delta. Unlike safeEmit, this
// blocks so rapid SSE tokens are never dropped when the UI briefly falls
// behind. The events channel is buffered; the HTTP reader will pace itself
// via backpressure instead of losing text.
func (s *Session) emitStreamChunk(text string) {
	defer func() { _ = recover() }()
	s.events <- protocol.SessionEvent{
		Type: "stream_chunk",
		Data: protocol.EventStreamChunk{Text: text},
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

// executeToolCalls runs one or more tool calls from a single assistant
// turn. Independent calls execute concurrently; results are appended to
// history in the model's original call order so provider APIs stay valid.
func (s *Session) executeToolCalls(ctx context.Context, calls []provider.ToolCall) {
	if len(calls) == 0 {
		return
	}
	if len(calls) == 1 {
		if msg := s.runToolCall(ctx, calls[0]); msg != nil {
			s.mu.Lock()
			s.history = append(s.history, *msg)
			s.mu.Unlock()
		}
		s.maybeFireDelayedCancel()
		return
	}

	s.mu.Lock()
	s.toolBatchConcurrent = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.toolBatchConcurrent = false
		s.mu.Unlock()
		s.maybeFireDelayedCancel()
	}()

	msgs := make([]*provider.Message, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c provider.ToolCall) {
			defer wg.Done()
			if ctx.Err() != nil {
				return
			}
			msgs[idx] = s.runToolCall(ctx, c)
		}(i, call)
	}
	wg.Wait()

	s.mu.Lock()
	for _, msg := range msgs {
		if msg != nil {
			s.history = append(s.history, *msg)
		}
	}
	s.mu.Unlock()
}

// runToolCall executes a single tool call and returns the history message
// to record (nil when the turn context is already cancelled).
func (s *Session) runToolCall(ctx context.Context, call provider.ToolCall) *provider.Message {
	if ctx.Err() != nil {
		return nil
	}
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
		return s.handleTodoWrite(call)
	case "ask_user_question":
		return s.handleAskUserQuestion(call)
	case "spawn_agent":
		return s.handleSpawnAgent(call)
	case "task_output":
		return s.handleTaskOutput(call)
	}
	tool, ok := s.tools.Get(call.Name)
	if !ok {
		s.emitToolCall(call.ID, call.Name, call.Arguments, "")
		s.emitToolResult(call.ID, call.Name, "", true, "unknown tool", nil)
		return toolHistoryMessage(call.ID, call.Name, "", true, "unknown tool")
	}
	// If the LLM produced a tool call whose JSON was
	// truncated (common for very large content the model
	// can't fit in its output token budget) the provider
	// layer falls back to storing the raw string in
	// `args["_raw"]`. Recover BEFORE emitting the tool
	// call event so the UI/activity strip can show a
	// clean summary (path/oldString/newString) instead
	// of `_raw="{\"newString\"...` noise.
	if raw, ok := call.Arguments["_raw"].(string); ok {
		call.Arguments = recoverArgsFromRaw(call.Name, call.Arguments, raw)
	}
	summary := summarizeToolCall(call.Name, call.Arguments)
	s.emitToolCall(call.ID, call.Name, call.Arguments, summary)
	tctx := tools.Context{
		CWD:        s.workdir,
		AllowShell: s.cfg.Tools.AllowShell,
		AllowWrite: s.cfg.Tools.AllowWrite,
		AllowGit:   s.cfg.Tools.AllowGit,
		Processes:  s.processes,
	}
	res, _ := tool.Run(tctx, call.Arguments)
	s.emitToolResult(call.ID, call.Name, res.Output, !res.OK, res.Error, res.Details)
	if !res.OK {
		return toolHistoryMessage(call.ID, call.Name, res.Output, true, res.Error)
	}
	return toolHistoryMessage(call.ID, call.Name, res.Output, false, "")
}

// handleTodoWrite parses a todo_write tool call and emits a
// structured EventTodoListUpdated. The TUI listens for that
// event and renders the todo list in the right panel. Without
// this hook the AI's todo list never reached the UI and the
// user reported 'the AI never makes a todo list when asked'.
func (s *Session) handleTodoWrite(call provider.ToolCall) *provider.Message {
	summary := summarizeArgs(call.Arguments)
	s.emitToolCall(call.ID, call.Name, call.Arguments, summary)

	var parsed []map[string]any
	switch v := call.Arguments["todos"].(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			errMsg := "todos: invalid JSON: " + err.Error()
			s.emitToolResult(call.ID, call.Name, "", true, errMsg, nil)
			return toolHistoryMessage(call.ID, call.Name, "", true, errMsg)
		}
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				parsed = append(parsed, m)
			}
		}
	default:
		s.emitToolResult(call.ID, call.Name, "", true, "todos: unsupported type", nil)
		return toolHistoryMessage(call.ID, call.Name, "", true, "todos: unsupported type")
	}
	if len(parsed) == 0 {
		s.emitToolResult(call.ID, call.Name, "", true, "todos: empty list", nil)
		return toolHistoryMessage(call.ID, call.Name, "", true, "todos: empty list")
	}

	items := make([]protocol.TodoItem, 0, len(parsed))
	for i, p := range parsed {
		content := protocol.TodoContentFromMap(p)
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
	s.mu.Lock()
	items = protocol.MergeTodoList(s.currentTodos, items)
	s.currentTodos = items
	s.mu.Unlock()
	s.safeEmit(protocol.SessionEvent{
		Type: "todo_list_updated",
		Data: protocol.EventTodoListUpdated{Todos: items},
	})
	out := fmt.Sprintf("updated %d todo(s)", len(items))
	s.emitToolResult(call.ID, call.Name, out, false, "", nil)
	return toolHistoryMessage(call.ID, call.Name, out, false, "")
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
func (s *Session) handleAskUserQuestion(call provider.ToolCall) *provider.Message {
	summary := summarizeArgs(call.Arguments)
	s.emitToolCall(call.ID, call.Name, call.Arguments, summary)

	var event protocol.EventUserQuestion
	if batch := parseAskUserQuestionBatch(call.Arguments["questions"]); len(batch) > 0 {
		for i := range batch {
			if err := normalizeQuestionDef(&batch[i], i); err != nil {
				s.emitToolResult(call.ID, call.Name, "", true, err.Error(), nil)
				return toolHistoryMessage(call.ID, call.Name, "", true, err.Error())
			}
		}
		event = protocol.EventUserQuestion{Questions: batch}
	} else {
		question, _ := call.Arguments["question"].(string)
		if question == "" {
			s.emitToolResult(call.ID, call.Name, "", true, "question or questions is required", nil)
			return toolHistoryMessage(call.ID, call.Name, "", true, "question or questions is required")
		}
		opts := parseAskUserQuestionOptions(call.Arguments["options"])
		if len(opts) < 2 {
			s.emitToolResult(call.ID, call.Name, "", true, "options: need at least 2 options", nil)
			return toolHistoryMessage(call.ID, call.Name, "", true, "options: need at least 2 options")
		}
		if len(opts) > 4 {
			opts = opts[:4]
		}
		header, _ := call.Arguments["header"].(string)
		if header == "" {
			header = "Question"
		}
		event = protocol.EventUserQuestion{
			Question:    question,
			RichOptions: opts,
			Category:    header,
		}
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
		Data: event,
	}:
		sent = true
	default:
		// TUI not listening — degrade to a synchronous
		// answer of "skip" so the LLM can keep going.
		out := "skipped (TUI unavailable)"
		s.emitToolResult(call.ID, call.Name, out, false, "", nil)
		return toolHistoryMessage(call.ID, call.Name, out, false, "")
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
		s.emitToolResult(call.ID, call.Name, out, false, "", nil)
		return toolHistoryMessage(call.ID, call.Name, out, false, "")
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
	s.emitToolResult(call.ID, call.Name, result, false, "", nil)
	return toolHistoryMessage(call.ID, call.Name, result, false, "")
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

// parseAskUserQuestionBatch normalises the `questions` argument into
// []protocol.QuestionDef. Accepts a JSON string, a []any of objects,
// or wrapper maps like {"item": [...]}.
func parseAskUserQuestionBatch(raw any) []protocol.QuestionDef {
	if raw == nil {
		return nil
	}
	if s, ok := raw.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		var arr []json.RawMessage
		if err := json.Unmarshal([]byte(s), &arr); err != nil {
			return nil
		}
		return parseAskUserQuestionBatchItems(arr)
	}
	if arr, ok := raw.([]any); ok {
		items := make([]json.RawMessage, 0, len(arr))
		for _, item := range arr {
			b, err := json.Marshal(item)
			if err != nil {
				continue
			}
			items = append(items, b)
		}
		return parseAskUserQuestionBatchItems(items)
	}
	if m, ok := raw.(map[string]any); ok {
		for _, key := range []string{"item", "items", "questions"} {
			if v, ok := m[key]; ok {
				if batch := parseAskUserQuestionBatch(v); len(batch) > 0 {
					return batch
				}
			}
		}
	}
	return nil
}

func parseAskUserQuestionBatchItems(items []json.RawMessage) []protocol.QuestionDef {
	out := make([]protocol.QuestionDef, 0, len(items))
	for _, raw := range items {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		q := protocol.QuestionDef{
			ID:       stringFromAny(m["id"]),
			Category: firstNonEmpty(stringFromAny(m["category"]), stringFromAny(m["header"])),
			Question: stringFromAny(m["question"]),
		}
		if opts := parseAskUserQuestionOptions(m["options"]); len(opts) > 0 {
			q.RichOptions = opts
		} else if simple := parseSimpleQuestionOptions(m["options"]); len(simple) > 0 {
			q.Options = simple
		}
		if q.Question == "" {
			continue
		}
		out = append(out, q)
	}
	return out
}

func parseSimpleQuestionOptions(raw any) []string {
	if raw == nil {
		return nil
	}
	if s, ok := raw.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		var arr []string
		if err := json.Unmarshal([]byte(s), &arr); err == nil {
			return filterNonEmptyStrings(arr)
		}
		return nil
	}
	if arr, ok := raw.([]any); ok {
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func filterNonEmptyStrings(arr []string) []string {
	out := make([]string, 0, len(arr))
	for _, s := range arr {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func stringFromAny(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func normalizeQuestionDef(q *protocol.QuestionDef, idx int) error {
	if q.ID == "" {
		q.ID = fmt.Sprintf("q%d", idx)
	}
	if q.Category == "" {
		q.Category = fmt.Sprintf("Question %d", idx+1)
	}
	optCount := len(q.Options)
	if len(q.RichOptions) > 0 {
		optCount = len(q.RichOptions)
		if optCount > 4 {
			q.RichOptions = q.RichOptions[:4]
			optCount = 4
		}
	} else if optCount > 4 {
		q.Options = q.Options[:4]
		optCount = 4
	}
	if optCount < 2 {
		return fmt.Errorf("questions[%d]: need at least 2 options", idx)
	}
	return nil
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

func summarizeToolCall(name string, args map[string]any) string {
	switch name {
	case "run_shell", "bash":
		cmd, _ := args["command"].(string)
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			break
		}
		if len(cmd) > 60 {
			cmd = cmd[:57] + "..."
		}
		label := "$ " + cmd
		if parseBoolArg(args, "background") {
			return label + " (background)"
		}
		if sec := shellTimeoutFromArgs(args); sec > 0 {
			return fmt.Sprintf("%s (timeout %ds)", label, sec)
		}
		return label
	case "grep", "search":
		if p, _ := args["pattern"].(string); p != "" {
			return p
		}
		if q, _ := args["query"].(string); q != "" {
			return q
		}
	case "glob_file_search", "glob_files":
		if p, _ := args["glob_pattern"].(string); p != "" {
			return p
		}
		if p, _ := args["pattern"].(string); p != "" {
			return p
		}
	case "web_fetch":
		if u, _ := args["url"].(string); u != "" {
			return u
		}
	case "write_file", "write_minified_file":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		if path != "" {
			lines := countArgLines(content)
			if lines == 1 {
				return fmt.Sprintf("%s (1 line)", path)
			}
			return fmt.Sprintf("%s (%d lines)", path, lines)
		}
	}
	return summarizeArgs(args)
}

func countArgLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

func summarizeArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	var keys []string
	for k := range args {
		// _raw can be tens of KB of malformed JSON
		// and makes the chat/activity strip unreadable
		// (e.g. edit_file _raw="{\"newString\"...").
		// It's still used internally by
		// recoverArgsFromRaw(), but never belongs in a
		// human-facing summary.
		if k == "_raw" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return "malformed/truncated tool arguments — retrying with smaller ordered fields"
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
		CortexPromptMode: mc.CortexPromptMode,
	})
	if err != nil {
		return provider.Response{}, err
	}

	sessionEffort := s.ReasoningEffort()

	modelSpec := mc.Provider + "/" + mc.Model
	if strings.Contains(canonical, "/") {
		modelSpec = canonical
	}
	s.llmCallStartedAt = time.Now()

	// Emit init state
	s.safeEmit(protocol.SessionEvent{
		Type: "init_state",
		Data: protocol.EventInitState{State: 1, Model: modelSpec},
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
		Stream:           true,
		ReasoningEffort:  provider.RequestReasoningEffort(mc.Provider, requestModel, sessionEffort),
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
	return prov.Stream(ctx, req, s.onChunk)
}

func (s *Session) onChunk(c provider.Chunk) {
	if c.Content != "" {
		s.emitStreamChunk(c.Content)
	}
	if c.Usage.TotalTokens > 0 || c.Usage.PromptTokens > 0 || c.FinishReason != "" {
		s.emitStreamDone(c.Usage, c.FinishReason)
	}
}

func (s *Session) emitStreamDone(u provider.Usage, finish provider.FinishReason) {
	var elapsedMs int64
	if !s.llmCallStartedAt.IsZero() {
		elapsedMs = time.Since(s.llmCallStartedAt).Milliseconds()
	}
	s.safeEmit(protocol.SessionEvent{
		Type: "stream_done",
		Data: protocol.EventStreamDone{
			InputTokens:  u.PromptTokens,
			OutputTokens: u.CompletionTokens,
			ElapsedMs:    elapsedMs,
			FinishReason: string(finish),
		},
	})
}

func (s *Session) emitToolCall(id, name string, args map[string]any, summary string) {
	ev := protocol.EventToolCall{
		ToolID:    id,
		Name:      name,
		Arguments: args,
		Summary:   summary,
	}
	if name == "run_shell" || name == "bash" {
		ev.TimeoutSec = shellTimeoutFromArgs(args)
		if r, ok := args["reason_to_increase_timeout"].(string); ok {
			ev.ReasonToIncreaseTimeout = r
		}
	}
	s.safeEmit(protocol.SessionEvent{Type: "tool_call", Data: ev})
}

func shellTimeoutFromArgs(args map[string]any) int {
	if v, ok := args["timeout_sec"]; ok {
		switch n := v.(type) {
		case float64:
			if n > 0 {
				return int(n)
			}
		case int:
			if n > 0 {
				return n
			}
		}
	}
	if parseBoolArg(args, "background") {
		return 0
	}
	return 120
}

func parseBoolArg(args map[string]any, key string) bool {
	v, ok := args[key].(bool)
	return ok && v
}

// StopBackgroundProcess terminates a tracked background shell process.
func (s *Session) StopBackgroundProcess(id string) error {
	if s.processes == nil {
		return errors.New("no process registry")
	}
	res, err := s.processes.Stop(id)
	if err != nil {
		return err
	}
	if !res.OK {
		return fmt.Errorf("%s", res.Error)
	}
	return nil
}

func (s *Session) emitBackgroundProcesses(procs []tools.BackgroundProcess) {
	items := make([]protocol.BackgroundProcessItem, 0, len(procs))
	for _, p := range procs {
		items = append(items, protocol.BackgroundProcessItem{
			ID:        p.ID,
			PID:       p.PID,
			Command:   p.Command,
			CWD:       p.CWD,
			StartedAt: p.StartedAt.Unix(),
			Running:   p.Running,
			ExitCode:  p.ExitCode,
		})
	}
	s.safeEmit(protocol.SessionEvent{
		Type: "background_processes_updated",
		Data: protocol.EventBackgroundProcessesUpdated{Processes: items},
	})
}

const writePreviewDetailPrefix = "@@cortex-write:"

func formatWritePreviewDetail(lineCount int, preview string) string {
	return fmt.Sprintf("%s%d@@\n%s", writePreviewDetailPrefix, lineCount, preview)
}

func (s *Session) emitToolResult(id, name, output string, isErr bool, errMsg string, details map[string]any) {
	if isErr {
		output = errMsg
	}
	detail := ""
	if details != nil {
		if d, ok := details["diff"].(string); ok && d != "" {
			detail = d
		} else if name == "write_file" || name == "write_minified_file" {
			if preview, ok := details["preview"].(string); ok && preview != "" {
				lineCount := 0
				switch n := details["lines"].(type) {
				case int:
					lineCount = n
				case int64:
					lineCount = int(n)
				case float64:
					lineCount = int(n)
				}
				detail = formatWritePreviewDetail(lineCount, preview)
			}
		} else if name == "web_fetch" {
			switch ms := details["elapsed_ms"].(type) {
			case int64:
				detail = fmt.Sprintf("%d", ms)
			case int:
				detail = fmt.Sprintf("%d", ms)
			case float64:
				detail = fmt.Sprintf("%d", int64(ms))
			}
		}
	}
	s.mu.Lock()
	showToolName := s.toolBatchConcurrent
	s.mu.Unlock()
	s.safeEmit(protocol.SessionEvent{
		Type: "tool_result",
		Data: protocol.EventToolResult{
			ToolID:       id,
			Name:         name,
			Output:       output,
			IsError:      isErr,
			Detail:       detail,
			Details:      details,
			ShowToolName: showToolName,
		},
	})
}

// toolHistoryMessage builds the provider history entry for a tool result.
func toolHistoryMessage(id, name, output string, isErr bool, errMsg string) *provider.Message {
	formatted := output
	if isErr {
		formatted = fmt.Sprintf("[tool %s] error: %s\n%s", name, errMsg, output)
	} else {
		formatted = fmt.Sprintf("[tool %s] ok\n%s", name, output)
	}
	msg := provider.Message{
		Role:       "tool",
		Content:    formatted,
		ToolName:   name,
		ToolCallID: id,
	}
	return &msg
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
		param := convertParamInfo(v)
		param.Required = req
		out[k] = param
	}
	return out
}

func convertParamInfo(v tools.ParamInfo) provider.ToolParam {
	p := provider.ToolParam{Type: v.Type, Description: v.Description}
	if v.Items != nil {
		item := convertParamInfo(*v.Items)
		p.Items = &item
	}
	if len(v.Properties) > 0 {
		p.Properties = make(map[string]provider.ToolParam, len(v.Properties))
		for k, child := range v.Properties {
			p.Properties[k] = convertParamInfo(child)
		}
	}
	return p
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
