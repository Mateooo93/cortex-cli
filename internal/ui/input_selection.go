package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const inputBoxTitleRows = 1

func inputContentInnerWidth(totalWidth int) int {
	w := totalWidth - 2*chatViewportBorderWidth - 2*chatViewportHPadding
	if w < 1 {
		return 1
	}
	return w
}

func (m *Model) inputSectionTopY() int {
	layout := m.currentLayout()
	y := layout.TabBarHeight + layout.ChatHeight
	sess := m.currentSession()
	if sess == nil {
		return y
	}
	if sess.attachmentPanel.IsVisible() {
		y += sess.attachmentPanel.Count() + 3
	}
	if sess.historyPanel.IsVisible() {
		y += sess.historyPanel.maxHeight + 2
	}
	return y
}

func (m *Model) inputSelectionAvailable() bool {
	if m.activeTab != TabKindChat {
		return false
	}
	sess := m.currentSession()
	if sess == nil || m.state == StateQuitConfirm {
		return false
	}
	if (sess.agentState == StateUserQuestion || sess.agentState == StateConfirmPending) && sess.questionPanel.IsVisible() {
		return false
	}
	return true
}

func (m *Model) inputInnerBounds() (top, bottom, left, right int, ok bool) {
	if !m.inputSelectionAvailable() {
		return 0, 0, 0, 0, false
	}
	sectionTop := m.inputSectionTopY()
	lines := len(m.inputTextDisplayLines(m.currentSession()))
	if lines < 1 {
		lines = 1
	}
	top = sectionTop + inputBoxTitleRows
	bottom = top + lines
	left = chatViewportBorderWidth + chatViewportHPadding
	right = m.width - chatViewportBorderWidth - chatViewportHPadding
	return top, bottom, left, right, true
}

func (m *Model) mouseInInputInner(x, y int) bool {
	top, bottom, left, right, ok := m.inputInnerBounds()
	if !ok {
		return false
	}
	return x >= left && x < right && y >= top && y < bottom
}

// inputTextDisplayLines expands the raw textarea value for hit-testing without
// calling textarea.View(), which can panic on uninitialized models in tests.
func (m *Model) inputTextDisplayLines(sess *SessionState) []string {
	if sess == nil {
		return nil
	}
	val := sess.input.Value()
	if val == "" {
		return []string{" "}
	}
	raw := strings.Split(strings.TrimSuffix(val, "\n"), "\n")
	return expandLinesToVisualRows(raw, inputContentInnerWidth(m.width))
}

func (m *Model) inputDisplayLines(sess *SessionState) []string {
	if sess == nil {
		return nil
	}
	view := sess.input.View()
	if view == "" {
		view = " "
	}
	raw := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
	return expandLinesToVisualRows(raw, inputContentInnerWidth(m.width))
}

func (m *Model) mouseToInputCell(x, y int) (lineIdx, cellX int, ok bool) {
	top, bottom, left, right, ok := m.inputInnerBounds()
	if !ok || x < left || x >= right || y < top || y >= bottom {
		return 0, 0, false
	}
	return y - top, x - left, true
}

func (m *Model) clampInputLineIndex(sess *SessionState, lineIdx int) int {
	if lineIdx < 0 {
		return 0
	}
	lines := m.inputTextDisplayLines(sess)
	if len(lines) == 0 {
		return 0
	}
	max := len(lines) - 1
	if lineIdx > max {
		return max
	}
	return lineIdx
}

func (m *Model) beginInputSelection(x, y int) {
	sess := m.currentSession()
	if sess == nil {
		return
	}
	lineIdx, cellX, ok := m.mouseToInputCell(x, y)
	if !ok {
		m.clearInputSelection()
		return
	}
	sess.chatSel.clear()
	lineIdx = m.clampInputLineIndex(sess, lineIdx)
	sess.inputSel.active = true
	sess.inputSel.anchorLine = lineIdx
	sess.inputSel.anchorX = cellX
	sess.inputSel.endLine = lineIdx
	sess.inputSel.endX = cellX
}

func (m *Model) extendInputSelection(x, y int) {
	sess := m.currentSession()
	if sess == nil || !sess.inputSel.active {
		return
	}
	top, bottom, left, right, ok := m.inputInnerBounds()
	if !ok {
		return
	}
	lineIdx := y - top
	cellX := x - left
	if lineIdx < 0 {
		lineIdx = 0
	}
	lineIdx = m.clampInputLineIndex(sess, lineIdx)
	if cellX < 0 {
		cellX = 0
	}
	maxX := inputContentInnerWidth(m.width) - 1
	if cellX > maxX {
		cellX = maxX
	}
	if y >= bottom {
		lineIdx = m.clampInputLineIndex(sess, lineIdx)
	}
	if x >= right {
		cellX = maxX
	}
	sess.inputSel.endLine = lineIdx
	sess.inputSel.endX = cellX
}

func (m *Model) clearInputSelection() {
	if sess := m.currentSession(); sess != nil {
		sess.inputSel.clear()
	}
}

func (m *Model) renderInputView(sess *SessionState) string {
	if sess == nil {
		return ""
	}
	if !sess.inputSel.active {
		return sess.input.View()
	}
	lines := m.inputTextDisplayLines(sess)
	selStyle := lipglossSelectionStyle()
	lines = applyChatSelectionHighlight(lines, sess.inputSel, selStyle)
	return strings.Join(lines, "\n")
}

func lipglossSelectionStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(lipgloss.Color("#2A4A7F")).Foreground(lipgloss.Color("#FFFFFF"))
}