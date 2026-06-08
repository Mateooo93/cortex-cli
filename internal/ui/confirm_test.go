package ui

import (
	"testing"
)

func TestQuitDialog_UsesSettingsViewportBorder(t *testing.T) {
	s := NewStyles(true)

	quitBorder := quitDialogStyle(s).GetBorderTopForeground()
	viewportBorder := s.ViewportFocusedStyle.GetBorderTopForeground()
	paletteBorder := s.CommandPaletteStyle.GetBorderTopForeground()

	if quitBorder != viewportBorder {
		t.Fatalf("quit border %v != settings viewport %v", quitBorder, viewportBorder)
	}
	if quitBorder == paletteBorder {
		t.Fatal("quit dialog should not use command palette border color")
	}
	if !quitDialogStyle(s).GetBorderTop() {
		t.Fatal("quit dialog should render a full top border for centered overlay")
	}
}