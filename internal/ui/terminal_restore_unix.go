//go:build !windows

package ui

import (
	"os"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-isatty"
)

// RestoreTerminal writes escape sequences that return the terminal to a usable
// state after the TUI exits. Bubble Tea normally does this on graceful shutdown,
// but this is a safety net when shutdown is bypassed (e.g. a raw os.Exit) or
// when cleanup races with a signal.
func RestoreTerminal() {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return
	}
	_, _ = os.Stdout.WriteString(
		ansi.ResetModeAltScreenSaveCursor +
			ansi.ResetModeAltScreen +
			ansi.ShowCursor +
			ansi.ResetModeMouseButtonEvent +
			ansi.ResetModeMouseAnyEvent +
			ansi.ResetModeMouseExtSgr +
			ansi.ResetModeBracketedPaste +
			ansi.ResetModifyOtherKeys +
			ansi.KittyKeyboard(0, 1),
	)
}