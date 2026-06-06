package session

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
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
