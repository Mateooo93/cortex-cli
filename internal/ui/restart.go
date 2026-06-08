package ui

import "sync"

// pendingRestart holds the binary path + argv for a post-quit re-exec.
// We quit the TUI first so bubbletea restores the terminal, then main()
// execs the freshly installed binary. Trying to syscall.Exec from inside
// the running TUI leaves the terminal in a bad state and users end up
// pressing Enter before the new session starts.
var pendingRestart struct {
	mu   sync.Mutex
	exe  string
	args []string
}

func setPendingRestart(exe string, args []string) {
	pendingRestart.mu.Lock()
	defer pendingRestart.mu.Unlock()
	pendingRestart.exe = exe
	pendingRestart.args = append([]string(nil), args...)
}

// TakePendingRestart returns the restart target queued by execSelfCmd.
// The bool is false when no restart was requested.
func TakePendingRestart() (exe string, args []string, ok bool) {
	pendingRestart.mu.Lock()
	defer pendingRestart.mu.Unlock()
	if pendingRestart.exe == "" {
		return "", nil, false
	}
	exe = pendingRestart.exe
	args = append([]string(nil), pendingRestart.args...)
	pendingRestart.exe = ""
	pendingRestart.args = nil
	return exe, args, true
}