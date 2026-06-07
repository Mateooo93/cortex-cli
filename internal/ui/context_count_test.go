package ui

import (
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/daemon"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
)

// TestContextCount_UsesMaxNotSum pins the context-window
// counting fix. The user reported: "context window fill
// up abnormaly quickly(its probable a counting issue)".
// The root cause: the streaming API reports the
// CURRENT turn's prompt size (which is the entire
// conversation history + new turn). Accumulating
// per-turn prompt tokens across 10 turns gives a
// 1+2+3+…+10 = 55x overestimate. The correct model is
// the MAX prompt size seen so far, which monotonically
// approaches the true context size.
func TestContextCount_UsesMaxNotSum(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	daemon.SetGlobalConfigLoader(func() *cortexconfig.Config { return cortexCfg })

	m := NewModel(cfg, cortexCfg, nil, false, "", true, true)
	sess := m.currentSession()

	// Simulate 5 streaming turn completions, each with
	// a growing prompt (1k, 2k, 3k, 4k, 5k tokens).
	// We need to find the right session and call
	// applyEventToSession directly (the UI's main
	// event loop wraps this in a sessionEventMsg).
	idx, _ := m.findSessionByDaemonID(m.currentSession().daemonSessionID)
	if idx < 0 {
		t.Fatal("could not find current session")
	}
	for _, sz := range []int64{1000, 2000, 3000, 4000, 5000} {
		m.applyEventToSession(idx, protocol.SessionEvent{
			Type: "event.stream_done",
			Data: protocol.EventStreamDone{
				InputTokens:  sz,
				OutputTokens: 200, // additive
			},
		})
	}
	sess = m.currentSession()

	// The correct context size is 5000 (the largest
	// prompt seen). The bug (accumulation) would give
	// 15000.
	if sess.inputTokens != 5000 {
		t.Errorf("context size = %d, want 5000 (the max — not 15000 from accumulation)", sess.inputTokens)
	}

	// Output tokens are additive (the model emits new
	// tokens each turn). 5 turns × 200 = 1000.
	if sess.outputTokens != 1000 {
		t.Errorf("output tokens = %d, want 1000 (additive across turns)", sess.outputTokens)
	}
}

// TestContextCount_ShrinksWhenModelResets pins the
// edge case: if the model reports a smaller prompt
// (e.g. after a context-window reset or summary), the
// counter should drop, not stay at the old max. The
// previous implementation set the counter to the max
// which means compaction never visibly shrinks the
// context bar.
func TestContextCount_ShrinksWhenModelResets(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	daemon.SetGlobalConfigLoader(func() *cortexconfig.Config { return cortexCfg })

	m := NewModel(cfg, cortexCfg, nil, false, "", true, true)
	sess := m.currentSession()

	// Build up to 8k context over 3 turns.
	idx2, _ := m.findSessionByDaemonID(m.currentSession().daemonSessionID)
	if idx2 < 0 {
		t.Fatal("could not find current session")
	}
	for _, sz := range []int64{2000, 5000, 8000} {
		m.applyEventToSession(idx2, protocol.SessionEvent{
			Type: "event.stream_done",
			Data: protocol.EventStreamDone{InputTokens: sz},
		})
	}
	sess = m.currentSession()
	if sess.inputTokens != 8000 {
		t.Fatalf("setup: context size = %d, want 8000", sess.inputTokens)
	}

	// Simulate a /compact that drops the context to 1k
	// (the summary + 4 kept messages). The next turn's
	// prompt should be 1k and the counter should drop.
	m.applyEventToSession(idx2, protocol.SessionEvent{
		Type: "event.stream_done",
		Data: protocol.EventStreamDone{InputTokens: 1000},
	})
	sess = m.currentSession()
	if sess.inputTokens != 1000 {
		t.Errorf("after compaction, context size = %d, want 1000 (counter should drop)", sess.inputTokens)
	}
}
