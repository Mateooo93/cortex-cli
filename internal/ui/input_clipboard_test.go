package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

func TestCtrlCDoesNotCopyChatSelection(t *testing.T) {
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
	sess.chatSel.active = true
	sess.chatSel.anchorLine = 0
	sess.chatSel.anchorX = 0
	sess.chatSel.endLine = 0
	sess.chatSel.endX = 3

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m2 := updated.(Model)
	if m2.state != StateQuitConfirm {
		t.Fatalf("expected quit confirm after ctrl+c, got state %v", m2.state)
	}
	if cmd != nil {
		t.Fatalf("expected no copy command from ctrl+c, got %T", cmd)
	}
}

func TestInputClipboardButtonHit(t *testing.T) {
	m := &Model{
		width:     120,
		height:    40,
		activeTab: TabKindChat,
		sessions:  []*SessionState{{agentState: StateWaitingForInput, focus: FocusEditor}},
	}
	top := m.inputSectionTopY()
	copyRect, pasteRect := inputClipboardButtonRects(top)
	if copyRect.Empty() || pasteRect.Empty() {
		t.Fatal("expected button rects")
	}
	copyX := copyRect.Min.X + 1
	pasteX := pasteRect.Min.X + 1
	y := copyRect.Min.Y
	if copyBtn, pasteBtn := m.inputClipboardButtonAt(copyX, y); !copyBtn || pasteBtn {
		t.Fatalf("copy=%v paste=%v at copy button", copyBtn, pasteBtn)
	}
	if copyBtn, pasteBtn := m.inputClipboardButtonAt(pasteX, y); copyBtn || !pasteBtn {
		t.Fatalf("copy=%v paste=%v at paste button", copyBtn, pasteBtn)
	}
}

func TestInputClipboardPromptIncludesButtons(t *testing.T) {
	prefix := inputClipboardPromptPrefix(inputBtnNone, true)
	if !containsPlain(prefix, "Copy") || !containsPlain(prefix, "Paste") {
		t.Fatalf("expected Copy and Paste in prompt prefix, got %q", prefix)
	}
	if !containsPlain(prefix, "❯") {
		t.Fatalf("expected chevron after buttons, got %q", prefix)
	}
}

func containsPlain(s, sub string) bool {
	return strings.Contains(stripANSI(s), sub)
}