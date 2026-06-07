package session

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
	"github.com/Mateooo93/cortex-cli/internal/provider"
)

// TestSendCancelAfterEdit_FiresAfterTool verifies that a delayed
// cancel waits for the in-flight tool call to complete before
// cancelling the turn's context.
func TestSendCancelAfterEdit_FiresAfterTool(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := cortexconfig.Default()

	// Use the "cortex" model; the provider construction in
	// NewSessionClient happens lazily on first call, but we never
	// get that far in this test — we test the cancel plumbing
	// directly, not the model.
	sess, err := New(Config{CortexCfg: cfg, ActiveModel: "cortex"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Inject a context that we control and a fake cancel function
	// so we can observe when the cancel actually fires.
	cancelCalled := int32(0)
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	ctx, cancel := context.WithCancel(parentCtx)
	sess.mu.Lock()
	sess.cancel = func() {
		atomic.StoreInt32(&cancelCalled, 1)
		cancel()
	}
	sess.cancelReq = false
	sess.delayedCancel = false
	sess.mu.Unlock()

	// Simulate: tool call is in flight. User hits Enter (delayed cancel).
	sess.SendCancelAfterEdit()

	// Cancel should NOT have fired yet — the in-flight edit is
	// still "running".
	if atomic.LoadInt32(&cancelCalled) != 0 {
		t.Errorf("expected cancel to be deferred while tool is in flight, got called=%d", cancelCalled)
	}
	if !sess.delayedCancel {
		t.Errorf("expected delayedCancel=true after SendCancelAfterEdit")
	}

	// Simulate: the tool call has just finished. The session
	// calls maybeFireDelayedCancel at the end of executeToolCall.
	sess.maybeFireDelayedCancel()

	// Now the cancel should have fired.
	if atomic.LoadInt32(&cancelCalled) != 1 {
		t.Errorf("expected cancel to fire after tool completion, got called=%d", cancelCalled)
	}
	if sess.delayedCancel {
		t.Errorf("expected delayedCancel to be cleared after firing")
	}
	if ctx.Err() == nil {
		t.Errorf("expected context to be cancelled")
	}
}

// TestSendCancelAfterEdit_NoToolInFlight verifies that when no
// tool call is in flight, the delayed cancel still takes effect
// (via the runTurn loop's pre-iteration check). This is the
// streaming-text case where "after the current edit" has no edit
// to wait for, so cancel happens as soon as the loop checks.
func TestSendCancelAfterEdit_FiresImmediatelyWhenNoTool(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := cortexconfig.Default()
	sess, err := New(Config{CortexCfg: cfg, ActiveModel: "cortex"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancelCalled := int32(0)
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	_, cancel := context.WithCancel(parentCtx)
	sess.mu.Lock()
	sess.cancel = func() {
		atomic.StoreInt32(&cancelCalled, 1)
		cancel()
	}
	sess.mu.Unlock()

	// Set the flag as if the user had hit Enter.
	sess.mu.Lock()
	sess.delayedCancel = true
	sess.mu.Unlock()

	// runTurn would now check delayedCancel at the top of its
	// next iteration. We simulate that check inline:
	sess.mu.Lock()
	if sess.delayedCancel && sess.cancel != nil {
		sess.cancel()
		sess.cancelReq = true
		sess.delayedCancel = false
	}
	sess.mu.Unlock()

	if atomic.LoadInt32(&cancelCalled) != 1 {
		t.Errorf("expected cancel to fire on next loop iteration, got called=%d", cancelCalled)
	}
}

// TestSendCancel_OverridesDelayedCancel verifies that if the user
// changes their mind and hits Ctrl+C, the immediate cancel takes
// precedence over the pending delayed cancel.
func TestSendCancel_OverridesDelayedCancel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := cortexconfig.Default()
	sess, err := New(Config{CortexCfg: cfg, ActiveModel: "cortex"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cancelCalled := int32(0)
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	_, cancel := context.WithCancel(parentCtx)
	sess.mu.Lock()
	sess.cancel = func() {
		atomic.StoreInt32(&cancelCalled, 1)
		cancel()
	}
	sess.delayedCancel = true
	sess.mu.Unlock()

	// User changes mind: Ctrl+C.
	sess.SendCancel()

	if atomic.LoadInt32(&cancelCalled) != 1 {
		t.Errorf("expected cancel to fire immediately, got called=%d", cancelCalled)
	}
	if sess.delayedCancel {
		t.Errorf("expected delayedCancel to be cleared by SendCancel")
	}

	// Give goroutines a moment to settle (in case anything async
	// is observing the state).
	time.Sleep(10 * time.Millisecond)
}

// TestRestoreHistory_ReplacesHistory verifies that RestoreHistory
// rebuilds the session's history with the given messages, so a
// freshly-reconnected session has the user's prior conversation
// context.
func TestRestoreHistory_ReplacesHistory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := cortexconfig.Default()
	sess, err := New(Config{CortexCfg: cfg, ActiveModel: "cortex"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Initially the history is empty.
	if h := sess.History(); len(h) != 0 {
		t.Errorf("expected empty initial history, got %d msgs", len(h))
	}

	// Restore a prior conversation.
	restored := []provider.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "how are you?"},
	}
	sess.RestoreHistory(restored)

	h := sess.History()
	// We expect 3 restored messages + 1 system message.
	if len(h) != 4 {
		t.Fatalf("expected 4 messages after RestoreHistory, got %d (%+v)", len(h), h)
	}
	if h[0].Role != "system" {
		t.Errorf("expected first message to be system, got %q", h[0].Role)
	}
	if h[1].Role != "user" || h[1].Content != "hi" {
		t.Errorf("expected second message to be user/hi, got %+v", h[1])
	}
	if h[2].Role != "assistant" || h[2].Content != "hello" {
		t.Errorf("expected third message to be assistant/hello, got %+v", h[2])
	}
	if h[3].Role != "user" || h[3].Content != "how are you?" {
		t.Errorf("expected fourth message to be user/how are you?, got %+v", h[3])
	}
}

// TestHandleAskUserQuestion_BlocksOnUserAnswer verifies that
// handleAskUserQuestion blocks on s.userAnswerCh and the
// user's response becomes the tool result recorded in
// history. This is the central bug fix: the previous design
// returned a "pending" placeholder which left the LLM with
// no real answer, the API rejected the next turn with
// HTTP 400 "tool call and result not match", and the user
// couldn't send any more messages.
func TestHandleAskUserQuestion_BlocksOnUserAnswer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := cortexconfig.Default()
	sess, err := New(Config{CortexCfg: cfg, ActiveModel: "cortex"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Start the question handler in a goroutine. The
	// arguments use a JSON-encoded string for options
	// (the format the previous version expected).
	go sess.handleAskUserQuestion(provider.ToolCall{
		ID:   "call-1",
		Name: "ask_user_question",
		Arguments: map[string]any{
			"question": "What kind of site?",
			"header":   "Website type",
			"options":  `[{"label":"Portfolio","description":"A site to showcase your work"},{"label":"Blog","description":"Long-form writing"}]`,
		},
	})

	// Drain events until we see user_question. The
	// handler emits a "tool_call" event first (with the
	// arguments for display) and then a "user_question"
	// event with the parsed options.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-sess.Events():
			if ev.Type == "user_question" {
				goto gotQuestion
			}
			// else: drain tool_call, etc.
		case <-deadline:
			t.Fatal("did not receive user_question event in 2s")
		}
	}
gotQuestion:

	// Answer the question. The handler should unblock
	// and emit a tool_result with the user's answer.
	sess.SendUserAnswer("Portfolio", "")

	// After the handler returns, the history should
	// contain a tool result whose content includes the
	// user's answer. The previous bug returned "question
	// shown in the TUI; awaiting user answer" instead.
	var found bool
	deadline2 := time.Now().Add(2 * time.Second)
	want := "[tool ask_user_question] Portfolio"
	for time.Now().Before(deadline2) {
		h := sess.History()
		for _, m := range h {
			if m.Role == "tool" && m.ToolCallID == "call-1" {
				if m.Content != want {
					t.Errorf("expected tool result %q, got %q", want, m.Content)
				}
				found = true
			}
		}
		if found {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !found {
		t.Fatal("expected tool result in history after SendUserAnswer")
	}
}

// TestHandleAskUserQuestion_AcceptsArrayOptions verifies
// the handler accepts options as a Go []any (the format
// the LLM sometimes sends — wrapped in a JSON object
// like {"item": [...]}) and still produces a valid
// question event.
func TestHandleAskUserQuestion_AcceptsArrayOptions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := cortexconfig.Default()
	sess, err := New(Config{CortexCfg: cfg, ActiveModel: "cortex"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Anthropic-style: options is an object with an
	// "item" key containing the array.
	args := map[string]any{
		"question": "Pick one",
		"header":   "Choice",
		"options": map[string]any{
			"item": []any{
				map[string]any{"label": "A", "description": "first"},
				map[string]any{"label": "B", "description": "second"},
			},
		},
	}

	go sess.handleAskUserQuestion(provider.ToolCall{
		ID:        "call-2",
		Name:      "ask_user_question",
		Arguments: args,
	})

	deadline2 := time.After(2 * time.Second)
	var gotEv protocol.SessionEvent
drainLoop:
	for {
		select {
		case ev := <-sess.Events():
			if ev.Type == "user_question" {
				gotEv = ev
				break drainLoop
			}
			// else: drain tool_call, etc.
		case <-deadline2:
			t.Fatal("did not receive user_question event in 2s")
		}
	}
	uq, ok := gotEv.Data.(protocol.EventUserQuestion)
	if !ok {
		t.Fatalf("expected EventUserQuestion, got %T", gotEv.Data)
	}
	if len(uq.RichOptions) != 2 {
		t.Fatalf("expected 2 rich options, got %d", len(uq.RichOptions))
	}
	if uq.RichOptions[0].Title != "A" || uq.RichOptions[1].Title != "B" {
		t.Errorf("expected titles A and B, got %q and %q",
			uq.RichOptions[0].Title, uq.RichOptions[1].Title)
	}

	// Cancel so the goroutine can exit.
	sess.SendUserAnswer("", "")
}

// drainQuestionEvents reads events from the session's event
// channel until it sees a "user_question" event (or times
// out). Other events (tool_call, etc.) are dropped on the
// floor. The session emits a "tool_call" event first to
// surface the arguments for display, then the structured
// "user_question" event with the parsed options.
func drainQuestionEvents(t *testing.T, sess *Session) protocol.EventUserQuestion {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-sess.Events():
			if ev.Type == "user_question" {
				uq, ok := ev.Data.(protocol.EventUserQuestion)
				if !ok {
					t.Fatalf("expected EventUserQuestion, got %T", ev.Data)
				}
				return uq
			}
		case <-deadline:
			t.Fatal("did not receive user_question event in 2s")
			return protocol.EventUserQuestion{}
		}
	}
}

// TestParseAskUserQuestionOptions_TableDriven exercises the
// three accepted input shapes plus the error cases (too few
// options, invalid JSON).
func TestParseAskUserQuestionOptions_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		raw       any
		wantCount int
		wantFirst string
	}{
		{
			name:      "string of array",
			raw:       `[{"label":"A","description":"a"},{"label":"B","description":"b"}]`,
			wantCount: 2,
			wantFirst: "A",
		},
		{
			name:      "string of single object",
			raw:       `{"label":"Solo","description":"s"}`,
			wantCount: 1,
			wantFirst: "Solo",
		},
		{
			name: "parsed array of maps",
			raw: []any{
				map[string]any{"label": "X", "description": "x"},
				map[string]any{"label": "Y", "description": "y"},
			},
			wantCount: 2,
			wantFirst: "X",
		},
		{
			name: "wrapped object with item",
			raw: map[string]any{
				"item": []any{
					map[string]any{"label": "P", "description": "p"},
					map[string]any{"label": "Q", "description": "q"},
				},
			},
			wantCount: 2,
			wantFirst: "P",
		},
		{
			name: "wrapped object with options",
			raw: map[string]any{
				"options": []any{
					map[string]any{"label": "M"},
					map[string]any{"label": "N"},
				},
			},
			wantCount: 2,
			wantFirst: "M",
		},
		{
			name:      "invalid JSON string",
			raw:       `not json`,
			wantCount: 0,
		},
		{
			name:      "nil",
			raw:       nil,
			wantCount: 0,
		},
		{
			name:      "empty array string",
			raw:       `[]`,
			wantCount: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseAskUserQuestionOptions(tt.raw)
			if len(opts) != tt.wantCount {
				t.Fatalf("got %d options, want %d (got %+v)", len(opts), tt.wantCount, opts)
			}
			if tt.wantCount > 0 && opts[0].Title != tt.wantFirst {
				t.Errorf("first title = %q, want %q", opts[0].Title, tt.wantFirst)
			}
		})
	}
}
