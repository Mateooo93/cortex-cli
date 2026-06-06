//go:build windows

package ui

import (
	tea "charm.land/bubbletea/v2"
)

// waitForResume is a no-op on Windows. Windows has no SIGCONT
// equivalent; instead, the tcell backend handles suspend/resume
// via its own kernel notifications.
func waitForResume() tea.Msg {
	// Block forever; Windows uses a different resume mechanism
	// (the bubble tea terminal layer raises WakeupMsg itself).
	<-make(chan struct{})
	return resumeFromSleepMsg{}
}
