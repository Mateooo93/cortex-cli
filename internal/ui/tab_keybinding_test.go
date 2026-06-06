package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// TestTabDuringStreamingQueuesMessage verifies that pressing Tab
// in the input editor while the agent is streaming/text/tool-
// executing and the input has text queues the message (Tab = queue
// for after the current turn).
func TestTabDuringStreamingQueuesMessage(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	sess.agentState = StateStreaming
	sess.input.SetValue("follow up message")
	sess.focus = FocusEditor

	// Simulate Tab keypress.
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	if sess.pendingInput == nil {
		t.Fatal("expected pendingInput to be set after Tab keypress during streaming")
	}
	if !sess.pendingInput.Queued {
		t.Errorf("expected pendingInput.Queued=true (Tab queue), got false")
	}
	if sess.input.Value() != "" {
		t.Errorf("expected input to be cleared after Tab queue, got %q", sess.input.Value())
	}
}

// TestTabDuringStreamingWithoutTextKeepsFocusCycling verifies that
// Tab without input text during streaming keeps its existing
// focus-cycling behavior (does NOT queue an empty message).
func TestTabDuringStreamingWithoutTextKeepsFocusCycling(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	sess.agentState = StateStreaming
	sess.input.SetValue("")
	sess.focus = FocusEditor

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	if sess.pendingInput != nil {
		t.Errorf("expected no pendingInput for empty input + Tab, got %+v", sess.pendingInput)
	}
	if sess.focus != FocusChat {
		t.Errorf("expected focus to cycle to chat when no text, got %v", sess.focus)
	}
}

// TestTabWhileWaitingCyclesFocus verifies that the existing
// focus-cycling Tab behavior is preserved when the agent is not
// running.
func TestTabWhileWaitingCyclesFocus(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	sess.agentState = StateWaitingForInput
	sess.input.SetValue("some text")
	sess.focus = FocusEditor

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// No queueing, no cancel -- just focus cycling.
	if sess.pendingInput != nil {
		t.Errorf("expected no pendingInput when waiting, got %+v", sess.pendingInput)
	}
	if strings.TrimSpace(sess.input.Value()) != "some text" {
		t.Errorf("expected input to be preserved when waiting, got %q", sess.input.Value())
	}
}
