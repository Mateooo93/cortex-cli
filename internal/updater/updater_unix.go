//go:build !windows

package updater

import (
	"syscall"
)

// windowsDetachedAttrs is a no-op on non-Windows. Kept as a
// stub so callers don't need build tags.
func windowsDetachedAttrs() *syscall.SysProcAttr {
	return nil
}
