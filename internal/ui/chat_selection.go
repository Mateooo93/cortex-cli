package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// visibleChatLines returns the chat lines currently shown inside the viewport,
// matching the scroll/padding logic in View.
func (m *Model) visibleChatLines(sess *SessionState, layout Layout) []string {
	var chatContent string
	if sess != nil {
		chatContent = buildRenderedChat(sess.chatMessages, m.styles, m.mdRenderer.width, sess.showThinking)
		if sess.showThinking && sess.thinkingRendered != "" {
			chatContent += sess.thinkingRendered + "\n"
		}
		if sess.assistantRendered != "" {
			chatContent += sess.assistantRendered
		} else if animFrame := sess.thinkingAnim.View(); animFrame != "" {
			chatContent += animFrame + "\n"
		}
	}
	contentHeight := layout.ChatHeight - 1
	if m.isWelcomeScreen(sess) {
		return welcomeViewportLines(m.mdRenderer.width, contentHeight, m.styles)
	}

	if chatContent == "" && !m.testMode {
		return welcomeViewportLines(m.mdRenderer.width, contentHeight, m.styles)
	}
	innerWidth := m.mdRenderer.width
	allLines := strings.Split(chatContent, "\n")

	visualRowStart := make([]int, len(allLines)+1)
	for i, line := range allLines {
		visualRowStart[i+1] = visualRowStart[i] + visualRows(line, innerWidth)
	}
	totalVisualRows := visualRowStart[len(allLines)]

	chatScrollOffset := 0
	if sess != nil {
		chatScrollOffset = sess.chatScrollOffset
	}
	endVisRow := totalVisualRows - chatScrollOffset
	if endVisRow < contentHeight {
		endVisRow = contentHeight
	}
	if endVisRow > totalVisualRows {
		endVisRow = totalVisualRows
	}

	endLogical := 0
	for endLogical < len(allLines) && visualRowStart[endLogical+1] <= endVisRow {
		endLogical++
	}
	accVisRows := 0
	startLogical := endLogical
	for startLogical > 0 {
		rows := visualRows(allLines[startLogical-1], innerWidth)
		if accVisRows+rows > contentHeight {
			break
		}
		accVisRows += rows
		startLogical--
	}

	chatLines := allLines[startLogical:endLogical]
	actualVisRows := visualRowStart[endLogical] - visualRowStart[startLogical]
	if padCount := contentHeight - actualVisRows; padCount > 0 {
		padLines := make([]string, padCount)
		for i := range padLines {
			padLines[i] = ""
		}
		chatLines = append(padLines, chatLines...)
	}

	if sess != nil && sess.chatScrollOffset > 0 && sess.client != nil {
		for _, sep := range turnSeparatorInfos(sess.chatMessages, m.styles, m.mdRenderer.width, sess.showThinking) {
			if sep.LineIdx >= startLogical && sep.LineIdx < endLogical {
				chatLines[sep.LineIdx-startLogical] = renderForkHintLine(m.mdRenderer.width+4, m.styles)
				break
			}
		}
	}
	return chatLines
}

// chatSelection tracks a drag-selection inside the chat message viewport.
// Line indices are relative to the visible chat lines slice; X is relative to
// the inner left edge of the chat content (past the border).
type chatSelection struct {
	active     bool
	anchorLine int
	anchorX    int
	endLine    int
	endX       int
}

func (s *chatSelection) clear() {
	*s = chatSelection{}
}

func (s chatSelection) normalized() (line0, line1, x0, x1 int) {
	line0, x0 = s.anchorLine, s.anchorX
	line1, x1 = s.endLine, s.endX
	if line0 > line1 || (line0 == line1 && x0 > x1) {
		line0, line1, x0, x1 = line1, line0, x1, x0
	}
	return line0, line1, x0, x1
}

func (m *Model) chatInnerBounds() (top, bottom, left, right int) {
	top, bottom, right = m.chatContentBounds()
	left = 1
	right = right - 1
	bottom = bottom - 1 // exclude bottom border row (BorderTop is false)
	return top, bottom, left, right
}

func (m *Model) mouseInChatInner(x, y int) bool {
	if m.activeTab != TabKindChat {
		return false
	}
	top, bottom, left, right := m.chatInnerBounds()
	return x >= left && x < right && y >= top && y < bottom
}

// mouseToChatCell maps terminal coordinates to a visible chat line index and
// inner-relative column.
func (m *Model) mouseToChatCell(x, y int) (lineIdx, cellX int, ok bool) {
	top, bottom, left, right := m.chatInnerBounds()
	if x < left || x >= right || y < top || y >= bottom {
		return 0, 0, false
	}
	lineIdx = y - top
	if lineIdx < 0 {
		return 0, 0, false
	}
	return lineIdx, x - left, true
}

func (m *Model) clampChatLineIndex(sess *SessionState, lineIdx int) int {
	if lineIdx < 0 {
		return 0
	}
	layout := m.currentLayout()
	lines := m.visibleChatLines(sess, layout)
	if len(lines) == 0 {
		return 0
	}
	max := len(lines) - 1
	if lineIdx > max {
		return max
	}
	return lineIdx
}

func (m *Model) beginChatSelection(x, y int) {
	sess := m.currentSession()
	if sess == nil {
		return
	}
	lineIdx, cellX, ok := m.mouseToChatCell(x, y)
	if !ok {
		m.clearChatSelection()
		return
	}
	lineIdx = m.clampChatLineIndex(sess, lineIdx)
	sess.chatSel.active = true
	sess.chatSel.anchorLine = lineIdx
	sess.chatSel.anchorX = cellX
	sess.chatSel.endLine = lineIdx
	sess.chatSel.endX = cellX
}

func (m *Model) extendChatSelection(x, y int) {
	sess := m.currentSession()
	if sess == nil || !sess.chatSel.active {
		return
	}
	top, bottom, left, right := m.chatInnerBounds()
	lineIdx := y - top
	cellX := x - left
	if lineIdx < 0 {
		lineIdx = 0
	}
	lineIdx = m.clampChatLineIndex(sess, lineIdx)
	if cellX < 0 {
		cellX = 0
	}
	maxX := m.mdRenderer.width - 1
	if cellX > maxX {
		cellX = maxX
	}
	if y >= bottom {
		lineIdx = m.clampChatLineIndex(sess, lineIdx)
	}
	if x >= right {
		cellX = maxX
	}
	sess.chatSel.endLine = lineIdx
	sess.chatSel.endX = cellX
}

func (m *Model) clearChatSelection() {
	if sess := m.currentSession(); sess != nil {
		sess.chatSel.clear()
	}
}

// applyChatSelectionHighlight styles visible chat lines that fall inside the
// current selection rectangle.
func applyChatSelectionHighlight(lines []string, sel chatSelection, style lipgloss.Style) []string {
	if !sel.active || len(lines) == 0 {
		return lines
	}
	line0, line1, x0, x1 := sel.normalized()
	out := make([]string, len(lines))
	for i, line := range lines {
		if i < line0 || i > line1 {
			out[i] = line
			continue
		}
		lineX0 := 0
		lineX1 := runewidth.StringWidth(stripANSI(line))
		if i == line0 {
			lineX0 = x0
			if lineX0 < 0 {
				lineX0 = 0
			}
		}
		if i == line1 {
			lineX1 = x1 + 1
			if lineX1 > runewidth.StringWidth(stripANSI(line)) {
				lineX1 = runewidth.StringWidth(stripANSI(line))
			}
		}
		out[i] = styleWidthRange(line, lineX0, lineX1, style)
	}
	return out
}

// chatSelectionPlainText extracts plain text from the selected region of the
// visible chat lines.
func chatSelectionPlainText(lines []string, sel chatSelection) string {
	if !sel.active || len(lines) == 0 {
		return ""
	}
	line0, line1, x0, x1 := sel.normalized()
	var parts []string
	for i, line := range lines {
		if i < line0 || i > line1 {
			continue
		}
		plain := stripANSI(line)
		lineX0 := 0
		lineX1 := runewidth.StringWidth(plain)
		if i == line0 {
			lineX0 = x0
			if lineX0 < 0 {
				lineX0 = 0
			}
		}
		if i == line1 {
			lineX1 = x1 + 1
			if lineX1 > runewidth.StringWidth(plain) {
				lineX1 = runewidth.StringWidth(plain)
			}
		}
		if lineX0 >= lineX1 {
			continue
		}
		parts = append(parts, plainWidthSlice(plain, lineX0, lineX1))
	}
	return strings.Join(parts, "\n")
}

func styleWidthRange(line string, x0, x1 int, style lipgloss.Style) string {
	if x0 >= x1 {
		return line
	}
	plainWidth := ansi.StringWidth(ansi.Strip(line))
	if x0 >= plainWidth {
		return line
	}
	if x1 > plainWidth {
		x1 = plainWidth
	}
	prefix := ansi.Cut(line, 0, x0)
	mid := ansi.Cut(line, x0, x1)
	suffix := ansi.TruncateLeft(line, x1, "")
	return prefix + selectionBackgroundPrefix(style) + mid + "\x1b[49m" + suffix
}

// selectionBackgroundPrefix returns an ANSI sequence that applies only the
// selection background so existing foreground styles stay visible underneath.
func selectionBackgroundPrefix(style lipgloss.Style) string {
	bg := lipgloss.NewStyle().Background(style.GetBackground()).Render("")
	if idx := strings.Index(bg, "\x1b[0m"); idx >= 0 {
		bg = bg[:idx]
	}
	return bg
}

func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inEscape := false
	for _, r := range s {
		if r == 0x1b {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '~' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func plainWidthSlice(s string, x0, x1 int) string {
	if x0 >= x1 {
		return ""
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		next := col + w
		if next <= x0 {
			col = next
			continue
		}
		if col >= x1 {
			break
		}
		b.WriteRune(r)
		col = next
	}
	return b.String()
}