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
	regions := tabBarHitRegions()
	if len(regions) != 4 {
		t.Fatalf("expected 4 tab regions, got %d", len(regions))
	}
	for _, r := range regions {
		for _, x := range []int{r.startX, r.endX} {
			got, ok := tabKindAtX(x)
			if !ok || got != r.kind {
				t.Fatalf("tabKindAtX(%d) = (%v,%v), want (%v,true)", x, got, ok, r.kind)
			}
		}
	}
	if _, ok := tabKindAtX(0); ok {
		t.Fatal("x=0 should miss all tabs")
	}
	if regions[0].endX+1 < regions[1].startX {
		if _, ok := tabKindAtX(regions[0].endX + 1); ok {
			t.Fatal("separator between tabs should not hit")
		}
	}
	settings := regions[2]
	if _, ok := tabKindAtX((settings.startX + settings.endX) / 2); !ok {
		t.Fatal("middle of settings tab should hit")
	}
}

func TestMouseClick_SettingsTabOpensSettings(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	m.width = 120
	m.height = 40

	settings := tabBarHitRegions()[2]
	updated, _ := m.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      (settings.startX + settings.endX) / 2,
		Y:      1,
	})
	if updated.(Model).activeTab != TabKindSettings {
		t.Fatalf("activeTab = %v, want TabKindSettings after clicking Settings tab", updated.(Model).activeTab)
	}
}

func TestHandleTabBarClick_SettingsTab(t *testing.T) {
	settings := tabBarHitRegions()[2]
	m := &Model{
		width:     100,
		height:    40,
		activeTab: TabKindChat,
		sessions:  []*SessionState{{}},
		mouseX:    (settings.startX + settings.endX) / 2,
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

func TestMouseMotion_KeepsDragSelectionWithoutButtonBit(t *testing.T) {
	setupPersistDir(t)
	m := NewModel(&config.Config{}, cortexconfig.Default(), nil, false, "", false, false)
	m.width = 100
	m.height = 40
	m.activeTab = TabKindChat
	top, _, left, _ := m.chatInnerBounds()
	updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: left + 2, Y: top})
	m = updated.(Model)
	sess := m.currentSession()
	if sess == nil || !sess.chatSel.active {
		t.Fatal("expected active selection after mouse down")
	}
	// Motion events during drag often carry Button=0.
	updated, _ = m.Update(tea.MouseMotionMsg{X: left + 8, Y: top})
	m = updated.(Model)
	if !sess.chatSel.active {
		t.Fatal("selection cleared after motion without button bit")
	}
	if sess.chatSel.endX < 6 {
		t.Fatalf("expected drag to extend selection, endX=%d", sess.chatSel.endX)
	}
}

func TestChatInnerBounds_AccountsForViewportPadding(t *testing.T) {
	m := &Model{
		width:     100,
		height:    40,
		activeTab: TabKindChat,
		sessions:  []*SessionState{{}},
	}
	_, _, left, right := m.chatInnerBounds()
	layout := m.currentLayout()
	wantInner := chatContentInnerWidth(layout)
	if right-left != wantInner {
		t.Fatalf("inner width = %d, want %d (left=%d right=%d)", right-left, wantInner, left, right)
	}
	if left != chatViewportBorderWidth+chatViewportHPadding {
		t.Fatalf("left = %d, want %d", left, chatViewportBorderWidth+chatViewportHPadding)
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