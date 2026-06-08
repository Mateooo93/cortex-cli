package ui

import (
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	xclip "golang.design/x/clipboard"
)

type pasteTarget int

const (
	pasteTargetNone pasteTarget = iota
	pasteTargetChat
	pasteTargetSettingsKey
	pasteTargetSettingsWizard
	pasteTargetSettingsProviderFilter
	pasteTargetSessions
	pasteTargetRightPanelKey
)

var (
	clipInitOnce sync.Once
	clipNativeOK bool
)

func initNativeClipboard() bool {
	clipInitOnce.Do(func() {
		clipNativeOK = xclip.Init() == nil
	})
	return clipNativeOK
}

// readClipboardText reads UTF-8 text from the system clipboard.
// Order: native (X11/Wayland) → xclip/wl-clipboard → empty.
func readClipboardText() (string, bool) {
	if initNativeClipboard() {
		if b := xclip.Read(xclip.FmtText); len(b) > 0 {
			return string(b), true
		}
	}
	if txt, err := clipboard.ReadAll(); err == nil && txt != "" {
		return txt, true
	}
	return "", false
}

// writeClipboardText writes UTF-8 text to the system clipboard.
func writeClipboardText(text string) bool {
	if text == "" {
		return false
	}
	if initNativeClipboard() {
		xclip.Write(xclip.FmtText, []byte(text))
		return true
	}
	return clipboard.WriteAll(text) == nil
}

func isPasteKey(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "ctrl+v", "ctrl+shift+v", "shift+insert":
		return true
	}
	return false
}

// pasteTarget reports where paste should land in the current UI state.
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
		if m.settingsActiveSection == 0 {
			return pasteTargetSettingsProviderFilter
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

// handlePasteKey reads the clipboard and inserts text into the focused input.
func (m Model) handlePasteKey() (Model, tea.Cmd) {
	if text, ok := readClipboardText(); ok {
		return m.applyPasteText(text)
	}
	return m, requestClipboardOSC52Cmd()
}

func requestClipboardOSC52Cmd() tea.Cmd {
	return func() tea.Msg {
		return tea.ReadClipboard()
	}
}

// copyToClipboardCmd copies text via native/OS clipboard or OSC52 fallback.
func copyToClipboardCmd(text string) tea.Cmd {
	if writeClipboardText(text) {
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

	case pasteTargetSettingsProviderFilter:
		var cmd tea.Cmd
		m.settingsProviderFilter, cmd = m.settingsProviderFilter.Update(paste)
		m.clampSettingsKeySel()
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