//go:build !windows

package ui

import (
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"
)

// waitForResume returns a tea.Cmd that blocks until the OS sends
// a "resume from suspend" signal. On Linux/macOS that's SIGCONT
// (delivered by systemd / pmset when the laptop wakes up).
//
// The returned cmd is invoked once at startup and again after
// every WakeupMsg so the UI re-attaches to the terminal cleanly
// after a long sleep.
func waitForResume() tea.Msg {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGCONT)
	<-sigCh
	signal.Stop(sigCh)
	return resumeFromSleepMsg{}
}
