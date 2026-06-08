package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// setupPersistDir makes the ~/.cortex directory exist so the
// persistSessions() call inside submitFromInput can write the
// sessions.json + chat files without erroring.
func setupPersistDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return dir
}

func TestSubmitFromInput_TabQueuesWithoutCancel(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	sess.agentState = StateStreaming
	sess.input.SetValue("fix the bug")

	_, _ = m.submitFromInput(sess, true)

	if sess.pendingInput == nil {
		t.Fatal("expected pendingInput to be set after Tab queue")
	}
	if !sess.pendingInput.Queued {
		t.Errorf("expected pendingInput.Queued=true (Tab queue), got false")
	}
	if sess.pendingInput.text != "fix the bug" {
		t.Errorf("pendingInput.text: got %q, want %q", sess.pendingInput.text, "fix the bug")
	}
	if sess.input.Value() != "" {
		t.Errorf("expected input to be cleared after queue, got %q", sess.input.Value())
	}
	if !strings.Contains(sess.input.Placeholder, "Queued") {
		t.Errorf("expected placeholder to show queued badge, got %q", sess.input.Placeholder)
	}
}

func TestSubmitFromInput_EnterQueuesWithDelayedCancel(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	sess.agentState = StateToolExecuting
	sess.input.SetValue("please stop and try this")

	_, _ = m.submitFromInput(sess, false)

	if sess.pendingInput == nil {
		t.Fatal("expected pendingInput to be set after Enter (delayed cancel)")
	}
	if sess.pendingInput.Queued {
		t.Errorf("expected pendingInput.Queued=false (Enter with delayed cancel), got true")
	}
	if !strings.Contains(sess.input.Placeholder, "Sending after current edit") {
		t.Errorf("expected placeholder to show 'sending after edit' badge, got %q", sess.input.Placeholder)
	}
}

func TestSubmitFromInput_EmptyInputIsNoOp(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	sess.agentState = StateStreaming
	sess.input.SetValue("   ")

	_, _ = m.submitFromInput(sess, true)
	_, _ = m.submitFromInput(sess, false)

	if sess.pendingInput != nil {
		t.Errorf("expected no pendingInput for whitespace input, got %+v", sess.pendingInput)
	}
}

func TestSubmitFromInput_RendersUserMessageInChat(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	preCount := len(sess.chatMessages)
	sess.agentState = StateStreaming
	sess.input.SetValue("first message")

	_, _ = m.submitFromInput(sess, true)

	if got := len(sess.chatMessages); got != preCount+1 {
		t.Fatalf("expected %d chat messages after submit, got %d", preCount+1, got)
	}
	last := sess.chatMessages[len(sess.chatMessages)-1]
	if last.Type != MsgUser {
		t.Errorf("expected last chat message to be user, got %v", last.Type)
	}
	if last.Text != "first message" {
		t.Errorf("last chat message text: got %q, want %q", last.Text, "first message")
	}
}

func TestPlaceholderForMode_ShowsQueuedBadge(t *testing.T) {
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]

	ph := m.placeholderForMode(sess)
	if !strings.Contains(ph, "Ask the agent anything") {
		t.Errorf("expected regular placeholder, got %q", ph)
	}

	sess.pendingInput = &pendingMsg{text: "fix the auth bug", Queued: true}
	ph = m.placeholderForMode(sess)
	if !strings.Contains(ph, "Queued") {
		t.Errorf("expected queued badge in placeholder, got %q", ph)
	}
	if !strings.Contains(ph, "fix the auth bug") {
		t.Errorf("expected queued message preview, got %q", ph)
	}

	sess.pendingInput = &pendingMsg{text: "stop and try this", Queued: false}
	ph = m.placeholderForMode(sess)
	if !strings.Contains(ph, "Sending after current edit") {
		t.Errorf("expected 'sending after edit' badge, got %q", ph)
	}
	if !strings.Contains(ph, "stop and try this") {
		t.Errorf("expected message preview, got %q", ph)
	}
}
