//go:build !windows

package tools

import (
	"os"
	"syscall"
	"time"
)

func stopProcessTree(pid int) {
	signalProcessGroup(pid, syscall.SIGTERM)
	time.Sleep(200 * time.Millisecond)
	signalProcessGroup(pid, syscall.SIGKILL)
}

func signalProcessGroup(pid int, sig syscall.Signal) {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		pgid = pid
	}
	_ = syscall.Kill(-pgid, sig)
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Signal(sig)
	}
}