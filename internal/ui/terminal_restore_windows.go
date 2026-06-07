//go:build windows

package ui

// RestoreTerminal is a no-op on Windows; console state is managed by the runtime.
func RestoreTerminal() {}