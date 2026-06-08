package ui

import (
	tea "charm.land/bubbletea/v2"
)

// functionKeyNum returns 1-4 for F1-F4, or 0 if not a function key we handle.
func functionKeyNum(msg tea.KeyPressMsg) int {
	switch msg.Code {
	case tea.KeyF1:
		return 1
	case tea.KeyF2:
		return 2
	case tea.KeyF3:
		return 3
	case tea.KeyF4:
		return 4
	}
	switch msg.String() {
	case "f1":
		return 1
	case "f2":
		return 2
	case "f3":
		return 3
	case "f4":
		return 4
	}
	return 0
}

// handleFunctionKey switches tabs via F1-F4.
func (m Model) handleFunctionKey(n int) (Model, tea.Cmd, bool) {
	if n < 1 || n > 4 {
		return m, nil, false
	}
	if m.state == StateQuitConfirm || m.state == StateSessionCloseConfirm {
		return m, nil, false
	}
	if m.updateOverlay.active || m.oauthAuthPending {
		return m, nil, false
	}

	var cmds []tea.Cmd

	switch n {
	case 1:
		if m.activeTab == TabKindSessions {
			ti := m.sessionsInput
			ti.SetValue("")
			m.sessionsInput = ti
			m.sessionsSelected = 0
			m.syncSessionsSelected()
			return m, m.sessionsInput.Focus(), true
		}
		m.activeTab = TabKindSessions
		m.sessionsSelected = 0
		m.syncSessionsSelected()
		m.clearChatSelection()
		return m, m.sessionsInput.Focus(), true

	case 2:
		m.activeTab = TabKindChat
		m.updateChatWidth()
		if sess := m.currentSession(); sess != nil {
			sess.unreadCount = 0
			sess.focus = FocusEditor
			m.sessionsInput.Blur()
			cmds = append(cmds, sess.input.Focus(), sess.thinkingAnim.Resume())
		}
		return m, tea.Batch(cmds...), true

	case 3:
		m.openSettingsTab()
		m.sessionsInput.Blur()
		m.clearChatSelection()
		return m, nil, true

	case 4:
		m.activeTab = TabKindWorkflows
		m.sessionsInput.Blur()
		m.clearChatSelection()
		return m, nil, true
	}

	return m, nil, false
}