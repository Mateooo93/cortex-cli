package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

func TestFunctionKeyF1FromChatTab(t *testing.T) {
	setupPersistDir(t)
	m := NewModel(&config.Config{}, cortexconfig.Default(), nil, false, "", false, false)
	m.activeTab = TabKindChat

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	if updated.(Model).activeTab != TabKindSessions {
		t.Fatalf("activeTab = %v, want TabKindSessions", updated.(Model).activeTab)
	}
}

func TestFunctionKeyF3FromChatInput(t *testing.T) {
	setupPersistDir(t)
	m := NewModel(&config.Config{}, cortexconfig.Default(), nil, false, "", false, false)
	m.activeTab = TabKindChat
	sess := m.currentSession()
	sess.agentState = StateWaitingForInput
	sess.input.Focus()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	if updated.(Model).activeTab != TabKindSettings {
		t.Fatalf("activeTab = %v, want TabKindSettings", updated.(Model).activeTab)
	}
}