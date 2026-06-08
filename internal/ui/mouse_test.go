package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
		{17, TabKindSessions, true},
		{19, TabKindChat, true},
		{30, TabKindChat, true},
		{33, TabKindSettings, true},
		{47, TabKindSettings, true},
		{60, 0, false},
	}
	for _, tc := range cases {
		got, ok := tabKindAtX(tc.x)
		if ok != tc.wantOK || got != tc.want {
			t.Fatalf("tabKindAtX(%d) = (%v,%v), want (%v,%v)", tc.x, got, ok, tc.want, tc.wantOK)
		}
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
		t.Fatal("top border should not be selectable")
	}
}

func TestApplyChatSelectionHighlight(t *testing.T) {
	sel := chatSelection{active: true, anchorX: 3, anchorY: 5, endX: 6, endY: 5}
	lines := []string{"hello world"}
	got := applyChatSelectionHighlight(lines, 5, 1, sel, lipgloss.NewStyle().Bold(true))
	if got[0] == lines[0] {
		t.Fatalf("expected styled selection, got %q", got[0])
	}
	if got := chatSelectionPlainText(lines, 5, 1, sel); got != "llo " {
		t.Fatalf("unexpected plain selection text %q", got)
	}
}