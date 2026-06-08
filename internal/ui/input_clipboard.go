package ui

import (
	"image"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type inputBtnHover int

const (
	inputBtnNone inputBtnHover = iota
	inputBtnCopy
	inputBtnPaste
)

const inputClipboardBtnGap = 1

// inputClipboardContentX is the first column inside the input box body (past
// border + padding).
const inputClipboardContentX = 2

func inputClipboardBtnStyle(hovered bool) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(colorDim)
	if hovered {
		s = s.Foreground(lipgloss.Color("15")).Background(colorSecondary).Bold(true)
	}
	return s
}

func renderInputClipboardBtn(label string, hovered bool) string {
	return inputClipboardBtnStyle(hovered).Render(" " + label + " ")
}

// inputClipboardPromptPrefix returns the first-line prompt: copy/paste buttons
// then the input chevron.
func inputClipboardPromptPrefix(hover inputBtnHover, focused bool) string {
	copyBtn := renderInputClipboardBtn("Copy", hover == inputBtnCopy)
	pasteBtn := renderInputClipboardBtn("Paste", hover == inputBtnPaste)
	chevron := "❯ "
	if !focused {
		chevron = lipgloss.NewStyle().Foreground(colorDim).Render(chevron)
	}
	return copyBtn + strings.Repeat(" ", inputClipboardBtnGap) + pasteBtn + strings.Repeat(" ", inputClipboardBtnGap) + chevron
}

func inputClipboardContinuationIndent(hover inputBtnHover, focused bool) string {
	w := lipgloss.Width(inputClipboardPromptPrefix(hover, focused))
	return strings.Repeat(" ", w)
}

func syncInputClipboardPrompt(sess *SessionState, hover inputBtnHover, focused bool) {
	prefix := inputClipboardPromptPrefix(hover, focused)
	indent := inputClipboardContinuationIndent(hover, focused)
	sess.input.SetPromptFunc(2, func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return prefix
		}
		return indent
	})
}

func resetInputPrompt(sess *SessionState) {
	sess.input.SetPromptFunc(2, func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return "❯ "
		}
		return "  "
	})
}

func (m *Model) showChatClipboardButtons() bool {
	if m.activeTab != TabKindChat || m.state == StateQuitConfirm {
		return false
	}
	sess := m.currentSession()
	if sess == nil {
		return false
	}
	if (sess.agentState == StateUserQuestion || sess.agentState == StateConfirmPending) && sess.questionPanel.IsVisible() {
		return false
	}
	return true
}

func (m *Model) syncChatInputPrompt() {
	sess := m.currentSession()
	if sess == nil {
		return
	}
	if m.showChatClipboardButtons() {
		syncInputClipboardPrompt(sess, m.inputBtnHover, sess.focus == FocusEditor)
	} else {
		resetInputPrompt(sess)
	}
}

// inputSectionTopY returns the screen row where the chat input box starts.
func (m *Model) inputSectionTopY() int {
	if m.activeTab != TabKindChat {
		return -1
	}
	layout := m.currentLayout()
	y := layout.TabBarHeight + layout.ChatHeight
	sess := m.currentSession()
	if sess != nil && sess.attachmentPanel.IsVisible() {
		y += sess.attachmentPanel.Count() + 3
	}
	if sess != nil && sess.historyPanel.IsVisible() {
		y += sess.historyPanel.maxHeight + 2
	}
	return y
}

func inputClipboardButtonRects(inputTopY int) (copyRect, pasteRect image.Rectangle) {
	if inputTopY < 0 {
		return image.Rectangle{}, image.Rectangle{}
	}
	y := inputTopY + 1 // first body row below the custom top border
	x := inputClipboardContentX
	copyW := lipgloss.Width(renderInputClipboardBtn("Copy", false))
	copyRect = image.Rect(x, y, x+copyW, y+1)
	x += copyW + inputClipboardBtnGap
	pasteW := lipgloss.Width(renderInputClipboardBtn("Paste", false))
	pasteRect = image.Rect(x, y, x+pasteW, y+1)
	return copyRect, pasteRect
}

func (m *Model) updateInputBtnHover(x, y int) {
	m.inputBtnHover = inputBtnNone
	if !m.showChatClipboardButtons() {
		return
	}
	top := m.inputSectionTopY()
	copyRect, pasteRect := inputClipboardButtonRects(top)
	if image.Pt(x, y).In(copyRect) {
		m.inputBtnHover = inputBtnCopy
		return
	}
	if image.Pt(x, y).In(pasteRect) {
		m.inputBtnHover = inputBtnPaste
	}
}

func (m *Model) inputClipboardButtonAt(x, y int) (copyBtn, pasteBtn bool) {
	if !m.showChatClipboardButtons() {
		return false, false
	}
	top := m.inputSectionTopY()
	copyRect, pasteRect := inputClipboardButtonRects(top)
	pt := image.Pt(x, y)
	return pt.In(copyRect), pt.In(pasteRect)
}

func (m Model) handleInputCopyClick() (Model, tea.Cmd) {
	sess := m.currentSession()
	if sess == nil {
		return m, nil
	}
	if sess.chatSel.active {
		if cmd := m.copyChatSelectionCmd(); cmd != nil {
			return m, cmd
		}
	}
	text := sess.input.Value()
	if text == "" {
		return m, m.emitStatusMsg("nothing to copy", StatusMsgInfo)
	}
	status := m.emitStatusMsg("copied to clipboard", StatusMsgInfo)
	if cmd := copyToClipboardCmd(text); cmd != nil {
		return m, tea.Batch(cmd, status)
	}
	return m, status
}

func (m Model) handleInputPasteClick() (Model, tea.Cmd) {
	if m.pasteTarget() != pasteTargetChat {
		return m, nil
	}
	return m.handlePasteKey()
}