package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

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

func TestLowercaseFScrollsWhenInputEmpty(t *testing.T) {
	setupPersistDir(t)
	m := NewModel(&config.Config{}, cortexconfig.Default(), nil, false, "", false, false)
	m.activeTab = TabKindChat
	sess := m.currentSession()
	sess.agentState = StateWaitingForInput
	sess.input.SetValue("")
	sess.chatScrollOffset = 10

	_, _ = m.Update(tea.KeyPressMsg{Code: 'f'})
	if sess.chatScrollOffset >= 10 {
		t.Fatalf("chatScrollOffset = %d, want scroll down from f when input empty", sess.chatScrollOffset)
	}
}