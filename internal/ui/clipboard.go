package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

type pasteTarget int

const (
	pasteTargetNone pasteTarget = iota
	pasteTargetChat
	pasteTargetSettingsKey
	pasteTargetSettingsWizard
	pasteTargetSessions
	pasteTargetRightPanelKey
)

// pasteTarget reports where Ctrl+V paste should land in the current UI state.
func (m Model) pasteTarget() pasteTarget {
	sess := m.currentSession()

	switch m.activeTab {
	case TabKindSettings:
		if m.settingsWizard.active {
			return pasteTargetSettingsWizard
		}
		if m.settingsInKeyInput {
			return pasteTargetSettingsKey
		}
	case TabKindSessions:
		return pasteTargetSessions
	case TabKindChat:
		if sess == nil {
			return pasteTargetNone
		}
		if sess.historyPanel.IsVisible() || sess.attachmentPanel.IsFocused() ||
			m.loginPicker.IsVisible() || m.modelPicker.IsVisible() ||
			sess.slashMenu.IsVisible() || m.commandPalette.IsVisible() {
			return pasteTargetNone
		}
		if sess.rightPanel.IsVisible() && sess.focus == FocusRightPanel && sess.rightPanel.mode == rpModeKeyInput {
			return pasteTargetRightPanelKey
		}
		if sess.agentState == StateWaitingForInput || sess.agentState == StatePlanReview ||
			sess.agentState == StateStreaming || sess.agentState == StateToolExecuting || sess.agentState == StatePlanExecuting {
			return pasteTargetChat
		}
	}
	return pasteTargetNone
}

// handlePasteKey tries the OS clipboard (xclip/wl-clipboard) and falls back to
// OSC52 when those tools are unavailable.
func (m Model) handlePasteKey() (Model, tea.Cmd) {
	if text, err := clipboard.ReadAll(); err == nil && text != "" {
		return m.applyPasteText(text)
	}
	return m, requestClipboardOSC52Cmd()
}

func requestClipboardOSC52Cmd() tea.Cmd {
	return func() tea.Msg {
		return tea.ReadClipboard()
	}
}

// copyToClipboardSync writes text via xclip/wl-clipboard. Returns true on success.
func copyToClipboardSync(text string) bool {
	return clipboard.WriteAll(text) == nil
}

// copyToClipboardCmd copies text via the OS clipboard or OSC52 fallback.
func copyToClipboardCmd(text string) tea.Cmd {
	if copyToClipboardSync(text) {
		return nil
	}
	return tea.SetClipboard(text)
}

// applyPasteText inserts clipboard text into the widget returned by pasteTarget.
func (m Model) applyPasteText(text string) (Model, tea.Cmd) {
	if text == "" {
		return m, nil
	}

	paste := tea.PasteMsg{Content: text}

	switch m.pasteTarget() {
	case pasteTargetSettingsWizard:
		w := m.settingsWizard
		var cmd tea.Cmd
		w.input, cmd = w.input.Update(paste)
		m.settingsWizard = w
		return m, cmd

	case pasteTargetSettingsKey:
		var cmd tea.Cmd
		m.settingsKeyInput, cmd = m.settingsKeyInput.Update(paste)
		return m, cmd

	case pasteTargetSessions:
		var cmd tea.Cmd
		m.sessionsInput, cmd = m.sessionsInput.Update(paste)
		return m, cmd

	case pasteTargetRightPanelKey:
		sess := m.currentSession()
		if sess == nil {
			return m, nil
		}
		var cmd tea.Cmd
		sess.rightPanel.keyInput, cmd = sess.rightPanel.keyInput.Update(paste)
		return m, cmd

	case pasteTargetChat:
		sess := m.currentSession()
		if sess == nil {
			return m, nil
		}
		var cmd tea.Cmd
		sess.input, cmd = sess.input.Update(paste)
		val := sess.input.Value()
		_, atts, _ := extractImageAttachments(val)
		if len(atts) > 0 {
			for i := range atts {
				sess.attachmentPanel.Add(atts[i])
			}
			stripped := imagePathPattern.ReplaceAllString(val, "")
			stripped = strings.TrimSpace(stripped)
			sess.input.SetValue(stripped)
		}
		newHeight := m.visualLineCount()
		if newHeight != sess.input.Height() {
			sess.input.SetHeight(newHeight)
		}
		sess.input.MoveToBegin()
		sess.input.MoveToEnd()
		return m, cmd
	}

	return m, nil
}