package ui

import (
	"testing"
)

// TestNewRightPanel_DefaultVisible verifies the right panel
// is visible by default — the user asked for it to be on
// from the first paint, with Ctrl+B to hide.
func TestNewRightPanel_DefaultVisible(t *testing.T) {
	rp := NewRightPanel()
	if !rp.IsVisible() {
		t.Error("right panel should be visible by default")
	}
	if rp.mode != rpModeInfo {
		t.Errorf("right panel should default to info mode, got %d", rp.mode)
	}
}

func TestRightPanel_Toggle(t *testing.T) {
	rp := NewRightPanel()
	rp.Toggle()
	if rp.IsVisible() {
		t.Error("after Toggle, panel should be hidden")
	}
	rp.Toggle()
	if !rp.IsVisible() {
		t.Error("after second Toggle, panel should be visible")
	}
}
