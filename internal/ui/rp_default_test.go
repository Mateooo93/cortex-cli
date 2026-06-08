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
	rp.Toggle(false)
	if rp.IsVisible() {
		t.Error("after Toggle, panel should be hidden")
	}
	rp.Toggle(false)
	if !rp.IsVisible() {
		t.Error("after second Toggle, panel should be visible")
	}
	if rp.mode != rpModeInfo {
		t.Errorf("Toggle without pending todos should reopen info mode, got %d", rp.mode)
	}
	rp.Toggle(true)
	if rp.IsVisible() {
		t.Error("third Toggle should hide panel again")
	}
	rp.Toggle(true)
	if rp.mode != rpModeTodos {
		t.Errorf("Toggle with pending todos should reopen todo mode, got %d", rp.mode)
	}
}
