package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

func TestIsPasteKey(t *testing.T) {
	tests := []struct {
		msg  tea.KeyPressMsg
		want bool
	}{
		{tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl}, true},
		{tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl | tea.ModShift}, true},
		{tea.KeyPressMsg{Code: tea.KeyInsert, Mod: tea.ModShift}, true},
		{tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, false},
	}
	for _, tt := range tests {
		if got := isPasteKey(tt.msg); got != tt.want {
			t.Errorf("isPasteKey(%v) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestRightClickOpensContextMenu(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	m.activeTab = TabKindChat
	sess := m.currentSession()
	if sess == nil {
		t.Fatal("expected session")
	}
	sess.agentState = StateWaitingForInput

	updated, _ := m.Update(tea.MouseClickMsg{
		Button: tea.MouseRight,
		X:      10,
		Y:      20,
	})
	m2 := updated.(Model)
	if !m2.contextMenu.active {
		t.Fatal("expected context menu to open on right-click")
	}
	if len(m2.contextMenu.items) == 0 {
		t.Fatal("expected at least Paste in context menu")
	}
	if m2.contextMenu.items[0].action != ctxActionPaste {
		t.Fatalf("first item = %q, want paste", m2.contextMenu.items[0].action)
	}
}