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

// TestRenderTabBar_FKeysAfterName verifies the tab bar renders
// each tab as "Name (F-key)" with the F-keybind in parentheses
// after the name, not as a highlighted badge before it. The
// user asked for plain text "(F1)" style with no special boxes
// or coloring — just quiet text that says what key to press.
//
// Because the name and the (F-key) suffix are rendered with
// different ANSI styles, we strip the escape sequences before
// checking the layout. We also check the order in the raw
// output to confirm the F-key comes AFTER the name.
func TestRenderTabBar_FKeysAfterName(t *testing.T) {
	s := NewStyles(true)
	bar := renderTabBar(TabKindChat, 120, s, true, false)
	// Strip ANSI escape sequences so the substring check
	// works regardless of style.
	plain := stripANSI(bar)
	for _, want := range []string{"Sessions (F1)", "Chat (F2)", "Workflows (F3)", "Settings (F4)"} {
		if !strings.Contains(plain, want) {
			t.Errorf("tab bar missing %q, got:\n%s", want, plain)
		}
	}
	// The old F-key badge style (" F1 " with surrounding
	// spaces) is gone — make sure we don't have any of
	// those lingering.
	for _, unwanted := range []string{"[F1]", "[F2]", "[F3]", "[F4]", " F1 ", " F2 ", " F3 ", " F4 "} {
		if strings.Contains(plain, unwanted) {
			t.Errorf("tab bar should not contain %q (old badge style), got:\n%s", unwanted, plain)
		}
	}
}

// TestRenderTabBar_AllFourTabs verifies the bar shows exactly
// four tabs (Sessions, Chat, Workflows, Settings) with the
// correct F-key for each. Regression test for a bug where the
// Workflows tab was missing.
func TestRenderTabBar_AllFourTabs(t *testing.T) {
	s := NewStyles(true)
	bar := renderTabBar(TabKindSettings, 120, s, true, false)
	plain := stripANSI(bar)
	for _, want := range []string{"Sessions", "Chat", "Workflows", "Settings", "(F1)", "(F2)", "(F3)", "(F4)"} {
		if !strings.Contains(plain, want) {
			t.Errorf("tab bar missing %q, got:\n%s", want, plain)
		}
	}
}

// stripANSI removes ANSI colour escape sequences so substring
// checks on styled output work regardless of which colors were
// applied. Regex matches the most common CSI sequences.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inEscape := false
	for _, r := range s {
		if r == 0x1b {
			inEscape = true
			continue
		}
		if inEscape {
			// CSI sequence ends at the first letter.
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '~' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestEscDuringStreamingResetsStateToWaiting verifies that
// pressing Esc while the agent is streaming resets the
// session state back to StateWaitingForInput so the user
// can submit a follow-up message. The previous version of
// the code only stopped the spinner and sent the cancel
// command, leaving agentState == StateStreaming — the
// submit path was a no-op on the next Enter press and the
// user reported "send a new one, nothing happens".
func TestEscDuringStreamingResetsStateToWaiting(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	sess.agentState = StateStreaming
	sess.thinkingAnim.Start()

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	if sess.agentState != StateWaitingForInput {
		t.Errorf("expected agentState StateWaitingForInput after Esc, got %v", sess.agentState)
	}
	if sess.thinkingAnim.active {
		t.Error("expected thinkingAnim to be stopped after Esc")
	}
}

// TestSubmitBeforeReconnectSurfacesWarning verifies that
// pressing Enter on a brand-new session (sess.client == nil
// because the reconnect goroutine hasn't finished) does
// NOT start the thinking anim. The previous version
// started the spinner even though no work was being done,
// so the user saw a forever-spinner with no message ever
// arriving.
func TestSubmitBeforeReconnectSurfacesWarning(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	sess.agentState = StateWaitingForInput
	// sess.client is nil — the reconnect hasn't completed
	// yet.
	sess.input.SetValue("hello world")
	sess.focus = FocusEditor

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// The spinner should NOT have been started.
	if sess.thinkingAnim.active {
		t.Error("thinkingAnim should NOT start when client is nil (reconnect in flight)")
	}
	// State should stay WaitingForInput so the user can
	// press Enter again once the reconnect finishes.
	if sess.agentState != StateWaitingForInput {
		t.Errorf("expected state StateWaitingForInput, got %v", sess.agentState)
	}
	// Text should be restored to the input.
	if sess.input.Value() != "hello world" {
		t.Errorf("expected input to be restored, got %q", sess.input.Value())
	}
}
