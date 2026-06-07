//go:build windows

package ui

import (
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
)

// execSelfCmd on Windows: syscall.Exec isn't available, so
// we just spawn the new binary as a child and exit the
// current TUI. The updater for Windows uses a
// helper-process flow (the running binary can't be
// renamed while it's executing), so by the time this
// runs the .new file has been moved into place. We sleep
// 250ms first to let any lingering Windows file lock
// release.
func (m *Model) execSelfCmd() tea.Cmd {
	m.persistSessions()
	exe, err := os.Executable()
	if err != nil {
		return tea.Quit
	}
	time.Sleep(250 * time.Millisecond)
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	_ = cmd.Start()
	time.Sleep(150 * time.Millisecond)
	return tea.Quit
}
