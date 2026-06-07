package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
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
	if chatContent == "" && !m.testMode {
		chatContent = renderWelcomeInline(m.mdRenderer.width, layout.ChatHeight-1, m.styles)
	}

	contentHeight := layout.ChatHeight - 1
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
// Coordinates are absolute terminal cells.
type chatSelection struct {
	active       bool
	anchorX      int
	anchorY      int
	endX         int
	endY         int
}

func (s *chatSelection) clear() {
	*s = chatSelection{}
}

func (s chatSelection) normalized() (x0, y0, x1, y1 int) {
	x0, y0 = s.anchorX, s.anchorY
	x1, y1 = s.endX, s.endY
	if y0 > y1 || (y0 == y1 && x0 > x1) {
		x0, y0, x1, y1 = x1, y1, x0, y0
	}
	return x0, y0, x1, y1
}

func (m *Model) chatInnerBounds() (top, bottom, left, right int) {
	top, bottom, right = m.chatContentBounds()
	left = 1
	right = right - 1
	top = top + 1
	bottom = bottom - 1
	return top, bottom, left, right
}

func (m *Model) mouseInChatInner(x, y int) bool {
	if m.activeTab != TabKindChat {
		return false
	}
	top, bottom, left, right := m.chatInnerBounds()
	return x >= left && x < right && y >= top && y < bottom
}

func (m *Model) beginChatSelection(x, y int) {
	sess := m.currentSession()
	if sess == nil {
		return
	}
	sess.chatSel.active = true
	sess.chatSel.anchorX = x
	sess.chatSel.anchorY = y
	sess.chatSel.endX = x
	sess.chatSel.endY = y
}

func (m *Model) extendChatSelection(x, y int) {
	sess := m.currentSession()
	if sess == nil || !sess.chatSel.active {
		return
	}
	sess.chatSel.endX = x
	sess.chatSel.endY = y
}

func (m *Model) clearChatSelection() {
	if sess := m.currentSession(); sess != nil {
		sess.chatSel.clear()
	}
}

// applyChatSelectionHighlight styles visible chat lines that fall inside the
// current selection rectangle.
func applyChatSelectionHighlight(lines []string, screenTopY, leftX int, sel chatSelection, style lipgloss.Style) []string {
	if !sel.active || len(lines) == 0 {
		return lines
	}
	x0, y0, x1, y1 := sel.normalized()
	out := make([]string, len(lines))
	for i, line := range lines {
		screenY := screenTopY + i
		if screenY < y0 || screenY > y1 {
			out[i] = line
			continue
		}
		lineX0 := 0
		lineX1 := runewidth.StringWidth(stripANSI(line))
		if screenY == y0 {
			lineX0 = x0 - leftX
			if lineX0 < 0 {
				lineX0 = 0
			}
		}
		if screenY == y1 {
			lineX1 = x1 - leftX + 1
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
func chatSelectionPlainText(lines []string, screenTopY, leftX int, sel chatSelection) string {
	if !sel.active || len(lines) == 0 {
		return ""
	}
	x0, y0, x1, y1 := sel.normalized()
	var parts []string
	for i, line := range lines {
		screenY := screenTopY + i
		if screenY < y0 || screenY > y1 {
			continue
		}
		plain := stripANSI(line)
		lineX0 := 0
		lineX1 := runewidth.StringWidth(plain)
		if screenY == y0 {
			lineX0 = x0 - leftX
			if lineX0 < 0 {
				lineX0 = 0
			}
		}
		if screenY == y1 {
			lineX1 = x1 - leftX + 1
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
	plain := stripANSI(line)
	if x0 >= runewidth.StringWidth(plain) {
		return line
	}
	prefix := plainWidthSlice(plain, 0, x0)
	mid := plainWidthSlice(plain, x0, x1)
	suffix := plainWidthSlice(plain, x1, runewidth.StringWidth(plain))
	return prefix + style.Render(mid) + suffix
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