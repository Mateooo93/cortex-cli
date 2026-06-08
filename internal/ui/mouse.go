package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// viewMouseMode keeps mouse capture on so the tab bar stays clickable.
// AllMotion is required for tab hover highlights without holding a button.
// Chat text selection is handled in-app inside the chat viewport.
func (m Model) viewMouseMode() tea.MouseMode {
	return tea.MouseModeAllMotion
}

// currentLayout mirrors the layout math used in View so hit-testing matches
// what is actually drawn.
func (m *Model) currentLayout() Layout {
	var panelHeights []int
	sess := m.currentSession()
	if sess != nil && sess.attachmentPanel.IsVisible() {
		panelHeights = append(panelHeights, sess.attachmentPanel.Count()+3)
	}
	if sess != nil && sess.historyPanel.IsVisible() {
		panelHeights = append(panelHeights, sess.historyPanel.maxHeight+2)
	}

	inputLines := m.visualLineCount()
	if sess != nil && (sess.agentState == StateUserQuestion || sess.agentState == StateConfirmPending) && sess.questionPanel.IsVisible() {
		inputLines = sess.questionPanel.Height()
	}

	layout := computeLayout(m.width, m.height, inputLines, panelHeights...)
	if sess != nil && sess.rightPanel.IsVisible() {
		layout.ChatWidth = m.width - sess.rightPanel.PanelWidth()
		if layout.ChatWidth < 10 {
			layout.ChatWidth = 10
		}
	}
	return layout
}

// chatContentBounds returns the half-open rectangle [top,bottom) x [0,right)
// covering the bordered chat viewport on the Chat tab.
func (m *Model) chatContentBounds() (top, bottom, right int) {
	layout := m.currentLayout()
	top = layout.TabBarHeight
	bottom = top + layout.ChatHeight
	right = layout.ChatWidth
	return top, bottom, right
}

// mouseInChatContent reports whether coordinates fall inside the chat box
// (including its border frame).
func (m *Model) mouseInChatContent(x, y int) bool {
	if m.activeTab != TabKindChat {
		return false
	}
	top, bottom, right := m.chatContentBounds()
	return x >= 0 && x < right && y >= top && y < bottom
}

func (m *Model) mouseInTabBar(y int) bool {
	return y >= 0 && y < m.currentLayout().TabBarHeight
}

func (m *Model) noteMousePosition(x, y int) {
	m.mouseX = x
	m.mouseY = y
}

// tabKindAtX returns which tab label was clicked. The bool is false when x
// falls outside all tab boxes. TabKindSessions is 0, so callers must use the
// bool — never compare the kind against zero.
func tabKindAtX(x int) (TabKind, bool) {
	type tabDef struct {
		name string
		key  string
		kind TabKind
	}
	defs := []tabDef{
		{"Sessions", "F1", TabKindSessions},
		{"Chat", "F2", TabKindChat},
		{"Settings", "F3", TabKindSettings},
	}

	visPos := 1
	for i, d := range defs {
		if i > 0 {
			visPos++
		}
		label := " " + d.name + " (" + d.key + ") "
		lw := len(label)
		x0 := visPos
		x1 := visPos + lw + 1
		if x >= x0 && x <= x1 {
			return d.kind, true
		}
		visPos += lw + 2
	}
	return 0, false
}

func (m *Model) handleTabBarClick() (Model, tea.Cmd) {
	if !m.mouseInTabBar(m.mouseY) {
		return *m, nil
	}
	kind, ok := tabKindAtX(m.mouseX)
	if !ok {
		return *m, nil
	}
	switch kind {
	case TabKindSessions:
		m.activeTab = TabKindSessions
		m.syncSessionsSelected()
		m.clearChatSelection()
		return *m, m.sessionsInput.Focus()
	case TabKindChat:
		m.activeTab = TabKindChat
		m.updateChatWidth()
		if sess := m.currentSession(); sess != nil {
			sess.unreadCount = 0
			sess.focus = FocusEditor
			return *m, sess.input.Focus()
		}
		return *m, nil
	case TabKindSettings:
		m.openSettingsTab()
		m.updateChatWidth()
		m.clearChatSelection()
		return *m, nil
	}
	return *m, nil
}

func (m *Model) handleChatMouseDown(x, y int) {
	if !m.mouseInChatInner(x, y) {
		m.clearChatSelection()
		return
	}
	m.beginChatSelection(x, y)
}

func (m *Model) handleChatMouseDrag(x, y int) {
	if !m.mouseInChatInner(x, y) {
		return
	}
	m.extendChatSelection(x, y)
}

// copyChatSelectionCmd copies the current chat drag-selection to the clipboard.
func (m *Model) copyChatSelectionCmd() tea.Cmd {
	sess := m.currentSession()
	if sess == nil || !sess.chatSel.active {
		return nil
	}
	layout := m.currentLayout()
	lines := m.visibleChatLines(sess, layout)
	if len(lines) == 0 {
		return nil
	}
	top, _, left, _ := m.chatInnerBounds()
	text := chatSelectionPlainText(lines, top, left, sess.chatSel)
	if text == "" {
		return nil
	}
	if err := clipboard.WriteAll(text); err != nil {
		return m.emitStatusMsg("copy failed: "+err.Error(), StatusMsgError)
	}
	return m.emitStatusMsg("copied selection", StatusMsgInfo)
}