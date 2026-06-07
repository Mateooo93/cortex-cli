package ui

import (
	"strings"
	"testing"
	"time"
)

func TestUpdateAnimProgress_AdvancesWithFrame(t *testing.T) {
	start := time.Now()
	p0 := updateAnimProgress(1, 0, start)
	p1 := updateAnimProgress(1, 14, start)
	if p1 <= p0 {
		t.Fatalf("expected progress to advance with frame: p0=%v p1=%v", p0, p1)
	}
	if p1 > 0.92 {
		t.Fatalf("expected progress capped before install step completes, got %v", p1)
	}
}

func TestRenderUpdateBrailleSpinner_ChangesWithFrame(t *testing.T) {
	a := renderUpdateBrailleSpinner(0, 40)
	b := renderUpdateBrailleSpinner(4, 40)
	if a == b {
		t.Fatal("expected braille spinner to change between frames")
	}
	if !strings.Contains(a, "⣾") && !strings.Contains(a, "⣽") {
		t.Fatalf("expected Heroku/bubbles braille glyph in spinner, got %q", stripANSI(a))
	}
}

func TestRenderUpdateMeterBar_ChangesWithFrame(t *testing.T) {
	start := time.Now()
	a := renderUpdateMeterBar(0, 1, start, 30)
	b := renderUpdateMeterBar(3, 1, start, 30)
	if a == b {
		t.Fatal("expected meter bar to change between frames")
	}
	if !strings.Contains(a, "█") && !strings.Contains(a, "░") {
		t.Fatalf("expected progress bar glyphs in bar, got %q", stripANSI(a))
	}
}

func TestCenterUpdateAnim_SymmetricSprite(t *testing.T) {
	sprite := "⣾"
	centered := centerUpdateAnim(sprite, 20)
	plain := stripANSI(centered)
	trimmed := strings.TrimSpace(plain)
	idx := strings.Index(plain, trimmed)
	if idx < 0 {
		t.Fatalf("sprite missing from centered line %q", plain)
	}
	leftPad := idx
	rightPad := len(plain) - idx - len(trimmed)
	diff := leftPad - rightPad
	if diff < -1 || diff > 1 {
		t.Fatalf("sprite not centered: leftPad=%d rightPad=%d line=%q", leftPad, rightPad, plain)
	}
}