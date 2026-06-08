//go:build windows

package tools

import (
	"os/exec"
	"strconv"
	"time"
)

func stopProcessTree(pid int) {
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T").Run()
	time.Sleep(200 * time.Millisecond)
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}