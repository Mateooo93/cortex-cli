//go:build !windows

package tools

import (
	"os/exec"
	"syscall"
)

func setShellProcGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}