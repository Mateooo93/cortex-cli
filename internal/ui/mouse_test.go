package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

func TestMouseInChatContent_OnlyOnChatTab(t *testing.T) {
	m := &Model{
		width:     100,
		height:    40,
		activeTab: TabKindChat,
		sessions:  []*SessionState{{}},
	}
	top, bottom, right := m.chatContentBounds()

	if !m.mouseInChatContent(right/2, top+1) {
		t.Fatal("expected coordinate inside chat viewport to hit chat content")
	}
	if m.mouseInChatContent(right/2, top-1) {
		t.Fatal("tab bar should not count as chat content")
	}
	if m.mouseInChatContent(right, top+1) {
		t.Fatal("coordinates outside chat width should miss")
	}
	if m.mouseInChatContent(right/2, bottom) {
		t.Fatal("row at bottom edge should miss chat content")
	}

	m.activeTab = TabKindSessions
	if m.mouseInChatContent(right/2, top+1) {
		t.Fatal("sessions tab should not expose chat selection region")
	}
}

func TestViewMouseMode_AllMotionForHover(t *testing.T) {
	m := Model{
		activeTab: TabKindChat,
		width:     100,
		height:    40,
		mouseX:    10,
		mouseY:    10,
		sessions:  []*SessionState{{}},
	}
	if got := m.viewMouseMode(); got != tea.MouseModeAllMotion {
		t.Fatalf("viewMouseMode = %v, want AllMotion for hover + clicks", got)
	}
}

func TestChatContentBounds_RespectRightPanel(t *testing.T) {
	m := &Model{
		width:     120,
		height:    40,
		activeTab: TabKindChat,
		sessions: []*SessionState{{
			rightPanel: func() RightPanel {
				rp := NewRightPanel()
				rp.visible = true
				return rp
			}(),
		}},
	}
	_, _, right := m.chatContentBounds()
	if right >= m.width {
		t.Fatalf("chat width should shrink when right panel is open, got %d", right)
	}
}

func TestTabKindAtX_MatchesRenderedLayout(t *testing.T) {
	cases := []struct {
		x      int
		want   TabKind
		wantOK bool
	}{
		{0, 0, false},
		{5, TabKindSessions, true},
		{12, TabKindSessions, true},
		{13, 0, false}, // separator between tabs
		{14, TabKindChat, true},
		{21, TabKindChat, true},
		{23, TabKindSettings, true},
		{34, TabKindSettings, true},
		{60, 0, false},
	}
	for _, tc := range cases {
		got, ok := tabKindAtX(tc.x)
		if ok != tc.wantOK || got != tc.want {
			t.Fatalf("tabKindAtX(%d) = (%v,%v), want (%v,%v)", tc.x, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestMouseClick_SettingsTabOpensSettings(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	m.width = 120
	m.height = 40

	updated, _ := m.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      28,
		Y:      1,
	})
	if updated.(Model).activeTab != TabKindSettings {
		t.Fatalf("activeTab = %v, want TabKindSettings after clicking Settings tab", updated.(Model).activeTab)
	}
}

func TestHandleTabBarClick_SettingsTab(t *testing.T) {
	m := &Model{
		width:     100,
		height:    40,
		activeTab: TabKindChat,
		sessions:  []*SessionState{{}},
		mouseX:    28,
		mouseY:    1,
	}
	updated, cmd := m.handleTabBarClick()
	if updated.activeTab != TabKindSettings {
		t.Fatalf("activeTab = %v, want TabKindSettings", updated.activeTab)
	}
	if cmd != nil {
		t.Fatalf("expected no focus cmd for settings tab, got %v", cmd)
	}
}

func TestHandleTabBarClick_SessionsTab(t *testing.T) {
	m := &Model{
		width:         100,
		height:        40,
		activeTab:     TabKindChat,
		sessions:      []*SessionState{{}},
		sessionsInput: newSessionsInput(),
		mouseX:        5,
		mouseY:        1,
	}
	updated, cmd := m.handleTabBarClick()
	if updated.activeTab != TabKindSessions {
		t.Fatalf("activeTab = %v, want TabKindSessions", updated.activeTab)
	}
	if cmd == nil {
		t.Fatal("expected focus command for sessions filter input")
	}
}

func TestMouseInChatInner_ExcludesBorder(t *testing.T) {
	m := &Model{
		width:     100,
		height:    40,
		activeTab: TabKindChat,
		sessions:  []*SessionState{{}},
	}
	top, _, left, _ := m.chatInnerBounds()
	if !m.mouseInChatInner(left, top) {
		t.Fatal("inner content cell should be selectable")
	}
	if m.mouseInChatInner(0, top) {
		t.Fatal("left border should not be selectable")
	}
	if m.mouseInChatInner(left, top-1) {
		t.Fatal("row above chat content should not be selectable")
	}
}

func TestApplyChatSelectionHighlight(t *testing.T) {
	sel := chatSelection{active: true, anchorLine: 0, anchorX: 2, endLine: 0, endX: 5}
	lines := []string{"hello world"}
	got := applyChatSelectionHighlight(lines, sel, lipgloss.NewStyle().Bold(true))
	if got[0] == lines[0] {
		t.Fatalf("expected styled selection, got %q", got[0])
	}
	if got := chatSelectionPlainText(lines, sel); got != "llo " {
		t.Fatalf("unexpected plain selection text %q", got)
	}
}

func TestApplyChatSelectionHighlightPreservesMarkdownStyle(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD080")).Bold(true).Render("hello") + " world"
	sel := chatSelection{active: true, anchorLine: 0, anchorX: 0, endLine: 0, endX: 2}
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#2A4A7F"))
	got := applyChatSelectionHighlight([]string{styled}, sel, selStyle)
	if !strings.Contains(got[0], "\x1b[") {
		t.Fatalf("expected ANSI styling to be preserved, got %q", got[0])
	}
	if strings.Contains(got[0], "hello world") && got[0] == styled {
		t.Fatalf("expected selection background overlay, got unchanged %q", got[0])
	}
}

func TestMouseToChatCellAccountsForContentOffset(t *testing.T) {
	m := &Model{
		width:     100,
		height:    40,
		activeTab: TabKindChat,
		sessions:  []*SessionState{{}},
	}
	top, _, left, _ := m.chatInnerBounds()
	lineIdx, cellX, ok := m.mouseToChatCell(left+4, top)
	if !ok {
		t.Fatal("expected mouse cell inside chat content")
	}
	if lineIdx != 0 {
		t.Fatalf("lineIdx = %d, want 0 for first content row", lineIdx)
	}
	if cellX != 4 {
		t.Fatalf("cellX = %d, want 4", cellX)
	}
}