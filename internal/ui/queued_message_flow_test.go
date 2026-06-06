package ui

import (
	"strings"
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// TestQueuedMessageGetsSentOnAgentDone verifies that a message
// queued via Tab (or Enter with delayed cancel) is captured into
// pendingInput and the input is cleared. The actual
// daemon.SendInput call is exercised in the integration test
// (it requires a connected model), but the input-side state
// transition is fully testable here.
func TestQueuedMessageGetsSentOnAgentDone(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	sess.agentState = StateStreaming

	// Simulate the user typing a follow-up and pressing Tab.
	sess.input.SetValue("what about the tests?")
	_, _ = m.submitFromInput(sess, true)

	if sess.pendingInput == nil {
		t.Fatal("expected pendingInput set after Tab queue")
	}
	if !sess.pendingInput.Queued {
		t.Errorf("expected Queued=true (Tab queue)")
	}
	if sess.input.Value() != "" {
		t.Errorf("expected input cleared after Tab queue, got %q", sess.input.Value())
	}
	if sess.agentState != StateStreaming {
		t.Errorf("expected agentState to stay streaming while queued, got %v", sess.agentState)
	}
	// The placeholder should reflect the queued state.
	if !strings.Contains(sess.input.Placeholder, "Queued") &&
		!strings.Contains(sess.input.Placeholder, "Sending after") {
		t.Errorf("expected placeholder to contain queue badge, got %q", sess.input.Placeholder)
	}
}
