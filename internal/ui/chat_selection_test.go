package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestExpandLineToVisualRowsSplitsWideStyledLine(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD080")).Render(strings.Repeat("a", 12))
	rows := expandLineToVisualRows(styled, 5)
	if len(rows) != 3 {
		t.Fatalf("expected 3 visual rows, got %d: %#v", len(rows), rows)
	}
}

func TestDisplayChatLinesMatchMouseLineIndex(t *testing.T) {
	m := Model{
		width:     40,
		height:    20,
		activeTab: TabKindChat,
		mdRenderer: NewMarkdownRenderer(34, true, lipgloss.NewStyle()),
		sessions: []*SessionState{{
			chatMessages: []ChatMessage{
				{Type: MsgAssistant, Rendered: strings.Repeat("x", 80) + "\n"},
			},
		}},
	}
	layout := m.currentLayout()
	lines := m.displayChatLines(m.currentSession(), layout)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped display lines, got %d", len(lines))
	}
	top, _, left, _ := m.chatInnerBounds()
	updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: left + 1, Y: top + len(lines) - 1})
	m = updated.(Model)
	sess := m.currentSession()
	if !sess.chatSel.active {
		t.Fatal("expected selection on last visible row")
	}
	if sess.chatSel.anchorLine != len(lines)-1 {
		t.Fatalf("anchorLine = %d, want %d", sess.chatSel.anchorLine, len(lines)-1)
	}
}

func TestCtrlCCopiesChatSelection(t *testing.T) {
	m := Model{
		width:     80,
		height:    30,
		activeTab: TabKindChat,
		mdRenderer: NewMarkdownRenderer(74, true, lipgloss.NewStyle()),
		sessions: []*SessionState{{
			chatMessages: []ChatMessage{
				{Type: MsgAssistant, Rendered: "hello world\n"},
			},
			chatSel: chatSelection{
				active:     true,
				anchorLine: 0,
				anchorX:    0,
				endLine:    0,
				endX:       4,
			},
		}},
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	out := updated.(Model)
	if out.state == StateQuitConfirm {
		t.Fatal("ctrl+c with active chat selection should copy, not open quit confirm")
	}
}