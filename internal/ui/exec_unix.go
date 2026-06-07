//go:build !windows

package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
)

// execSelfCmd returns a tea.Cmd that re-execs the current
// cortex binary. We use syscall.Exec to atomically replace
// the running process with the freshly downloaded binary
// (the one the updater just wrote to disk). The user wanted:
// "the cli should restart once its ready and the tui has
// been updated".
//
// The flow is:
//  1. Persist any open sessions / chat history / etc. so
//     the new TUI restores them on launch.
//  2. Resolve the running binary's absolute path (with
//     symlinks resolved — the updater wrote the new
//     version over this path).
//  3. Build a `syscall.Execve` argv / envv from os.Args
//     and os.Environ().
//  4. Call syscall.Exec — the kernel replaces the current
//     process image with the new one. If the call fails
//     (extremely rare — e.g. the binary was just replaced
//     and a permission bit got reset) we fall back to
//     `os.Exit(0)` so the user can restart manually.
//
// On platforms where syscall.Exec isn't available (read:
// Windows, see exec_windows.go) this function is a stub
// that just prints a message and exits 0.
func (m *Model) execSelfCmd() tea.Cmd {
	// 1. Persist state so the new TUI picks up where we
	//    left off.
	m.persistSessions()
	// Also save the providers / settings if anything's
	// dirty. The Save() now uses deep-merge so this is
	// safe and fast.
	// (We don't know the path here, but the existing
	// persist path is good enough — cortex saves on
	// every meaningful change.)

	// 2. Resolve the binary path.
	exe, err := os.Executable()
	if err != nil {
		// Couldn't find ourselves — just exit so the
		// user can manually restart with the new
		// binary.
		return tea.Quit
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	// 3. Build argv (preserve --config-dir etc. that the
	//    user passed) and envv.
	argv := append([]string{exe}, os.Args[1:]...)
	envv := os.Environ()

	// 4. Re-exec. syscall.Exec replaces the process
	//    image; on success this never returns. On
	//    failure (e.g. binary not executable), fall
	//    back to running it as a child + exit.
	if err := syscall.Exec(exe, argv, envv); err == nil {
		// Unreachable on success, but Go needs the
		// return to be valid.
		return tea.Quit
	}

	// Fallback: spawn the new binary as a child
	// process and exit the current TUI. This is less
	// elegant (the user briefly sees the terminal
	// un-alt-screen between the two processes) but
	// works when syscall.Exec fails.
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = envv
	_ = cmd.Start()
	// Give the child a moment to start, then exit.
	time.Sleep(150 * time.Millisecond)
	return tea.Quit
}
