//go:build windows

package updater

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"
)

// windowsDetachedAttrs returns SysProcAttr that detaches the
// child from our process group so it survives our exit. Used by
// the install helper.
func windowsDetachedAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// CREATE_NEW_PROCESS_GROUP = 0x00000200
		// DETACHED_PROCESS = 0x00000008
		CreationFlags: 0x00000008 | 0x00000200,
	}
}

// runUpdateHelper is the entry point for the helper process
// spawned by installOnWindows. It waits for the parent cortex
// process to release its lock on the binary, then performs the
// swap.
//
// argv[1] is the magic sentinel "__update_helper__" so the
// helper is only triggered when explicitly invoked (and never
// by accident if a user double-clicks the binary).
func runUpdateHelper() {
	if len(os.Args) < 4 || os.Args[1] != "__update_helper__" {
		return
	}
	currentExe := os.Args[2]
	newExe := os.Args[3]

	// Wait for the parent to exit. We poll for the file lock
	// by trying to rename the current binary to itself; if the
	// rename succeeds the lock is gone.
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "updater helper: timed out waiting for parent to exit")
			os.Exit(1)
		}
		// Best-effort rename-to-self; if the file is locked
		// (still running) this fails.
		if err := os.Rename(currentExe, currentExe+".waiting"); err == nil {
			_ = os.Rename(currentExe+".waiting", currentExe)
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Now do the swap.
	_ = os.RemoveAll(currentExe + ".old")
	if err := os.Rename(currentExe, currentExe+".old"); err != nil {
		fmt.Fprintf(os.Stderr, "updater helper: rename old: %v\n", err)
		os.Exit(1)
	}
	if err := os.Rename(newExe, currentExe); err != nil {
		fmt.Fprintf(os.Stderr, "updater helper: rename new: %v\n", err)
		_ = os.Rename(currentExe+".old", currentExe)
		os.Exit(1)
	}
	// Best-effort cleanup.
	_ = os.RemoveAll(newExe + ".helper")
}

// RunWithContext is a wrapper used by the helper entry point;
// not exported as Run for the helper.
func RunWithContext(ctx context.Context) error {
	runUpdateHelper()
	return nil
}
