package ui

import (
	tea "charm.land/bubbletea/v2"
)

const hoverProcess = 2

// rightPanelScreenBounds returns the screen rectangle of the right panel
// on the Chat tab.
func (m *Model) rightPanelScreenBounds() (left, top, right, bottom int, ok bool) {
	if m.activeTab != TabKindChat {
		return 0, 0, 0, 0, false
	}
	sess := m.currentSession()
	if sess == nil || !sess.rightPanel.IsVisible() {
		return 0, 0, 0, 0, false
	}
	layout := m.currentLayout()
	top = layout.TabBarHeight - 1
	bottom = top + layout.ChatHeight + 1
	left = m.width - sess.rightPanel.PanelWidth()
	right = m.width
	return left, top, right, bottom, true
}

func (m *Model) rightPanelContentLineAt(x, y int) (contentLine int, ok bool) {
	left, top, right, bottom, ok := m.rightPanelScreenBounds()
	if !ok || x < left || x >= right || y < top || y >= bottom {
		return 0, false
	}
	contentLine = y - top - 1
	if contentLine < 0 {
		return 0, false
	}
	return contentLine, true
}

func (m *Model) processHoverAt(x, y int) string {
	sess := m.currentSession()
	if sess == nil || sess.rightPanel.mode != rpModeInfo {
		return ""
	}
	line, ok := m.rightPanelContentLineAt(x, y)
	if !ok {
		return ""
	}
	pid, ok := sess.rightPanel.ProcessIDAtContentLine(line)
	if !ok {
		return ""
	}
	for _, p := range sess.backgroundProcesses {
		if p.ID == pid && p.Running {
			return pid
		}
	}
	return ""
}

func (m *Model) updateProcessHover(x, y int) {
	sess := m.currentSession()
	if sess == nil {
		return
	}
	sess.hoverProcessID = m.processHoverAt(x, y)
}

func (m *Model) handleRightPanelProcessClick(x, y int) (handled bool, cmds []tea.Cmd) {
	sess := m.currentSession()
	if sess == nil || sess.rightPanel.mode != rpModeInfo {
		return false, nil
	}
	pid := m.processHoverAt(x, y)
	if pid == "" {
		return false, nil
	}
	if sess.client == nil {
		return true, []tea.Cmd{m.emitStatusMsg("not connected", StatusMsgError)}
	}
	if err := sess.client.SendStopBackgroundProcess(pid); err != nil {
		return true, []tea.Cmd{m.emitStatusMsg(err.Error(), StatusMsgError)}
	}
	sess.hoverProcessID = ""
	return true, []tea.Cmd{m.emitStatusMsg("Stopped "+pid, StatusMsgInfo)}
}