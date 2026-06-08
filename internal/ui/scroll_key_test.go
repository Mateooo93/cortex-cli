package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

func TestLowercaseFTypesIntoEmptyPrompt(t *testing.T) {
	setupPersistDir(t)
	m := NewModel(&config.Config{}, cortexconfig.Default(), nil, false, "", false, false)
	m.activeTab = TabKindChat
	sess := m.currentSession()
	sess.agentState = StateWaitingForInput
	sess.input.SetValue("")
	sess.input.Focus()

	_, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if sess.input.Value() != "f" {
		t.Fatalf("input = %q, want %q (f must type even when prompt is empty)", sess.input.Value(), "f")
	}
}

func TestLowercaseFTypesIntoInputWhenNotEmpty(t *testing.T) {
	setupPersistDir(t)
	m := NewModel(&config.Config{}, cortexconfig.Default(), nil, false, "", false, false)
	m.activeTab = TabKindChat
	sess := m.currentSession()
	sess.agentState = StateWaitingForInput
	sess.input.SetValue("di")
	sess.input.Focus()

	_, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if !strings.Contains(sess.input.Value(), "f") {
		t.Fatalf("input = %q, want letter f typed (not stolen for scroll)", sess.input.Value())
	}
}

