//go:build windows

package ui

import (
	"os"

	tea "charm.land/bubbletea/v2"
)

// execSelfCmd quits the TUI and arranges for main() to spawn the updated
// binary after bubbletea restores the terminal.
func (m *Model) execSelfCmd() tea.Cmd {
	exe := m.updateOverlay.restartPath
	m.updateOverlay.restartPath = ""
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			return func() tea.Msg { return tea.Quit() }
		}
	}

	m.updateOverlay.active = false
	setPendingRestart(exe, os.Args[1:])

	return func() tea.Msg {
		m.persistSessions()
		return tea.Quit()
	}
}