package ui

import (
	"strings"
	"testing"
	"time"
)

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1k"},
		{1500, "1k"},    // integer truncation, not rounding
		{125000, "125k"},
	}
	for _, tt := range tests {
		got := formatTokenCount(tt.n)
		if got != tt.want {
			t.Errorf("formatTokenCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "00:00"},
		{30 * time.Second, "00:30"},
		{5*time.Minute + 30*time.Second, "05:30"},
		{1 * time.Hour, "01:00:00"},
		{1*time.Hour + 5*time.Minute + 30*time.Second, "01:05:30"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

// TestRenderKeybindHintMentionsTabAndEnter verifies the
// bottom-left keybind hint explicitly mentions the two keybinds
// the user asked for: Tab (queue) and Enter (send / interrupt).
func TestRenderKeybindHintMentionsTabAndEnter(t *testing.T) {
	s := NewStyles(true)
	hint := renderKeybindHint(120, s)
	if !strings.Contains(hint, "Enter") {
		t.Errorf("expected hint to mention Enter, got %q", hint)
	}
	if !strings.Contains(hint, "Tab") {
		t.Errorf("expected hint to mention Tab, got %q", hint)
	}
	if !strings.Contains(hint, "queue") {
		t.Errorf("expected hint to mention queue, got %q", hint)
	}
	if !strings.Contains(hint, "send") {
		t.Errorf("expected hint to mention send, got %q", hint)
	}
}

// TestRenderStatusBarShowsHintWhenIdle verifies the keybind hint
// is rendered on the status bar's second line when no transient
// status message is active. The hint is the bottom-left footer
// the user asked for.
func TestRenderStatusBarShowsHintWhenIdle(t *testing.T) {
	s := NewStyles(true)
	bar := renderStatusBar(120, true, false, StatusMessage{}, s)
	lines := strings.Split(bar, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines in status bar, got %d", len(lines))
	}
	// The second line is the hint when no status message is set.
	if !strings.Contains(lines[0], "queue") {
		t.Errorf("expected idle status bar to show keybind hint with 'queue', got %q", lines[0])
	}
}

// TestRenderStatusBarShowsStatusMsgWhenPresent verifies the
// status message takes precedence over the keybind hint when
// one is active (the user wants to see a transient warning/error).
func TestRenderStatusBarShowsStatusMsgWhenPresent(t *testing.T) {
	s := NewStyles(true)
	bar := renderStatusBar(120, true, false, StatusMessage{Text: "out of API credits", Kind: StatusMsgError}, s)
	if !strings.Contains(bar, "out of API credits") {
		t.Errorf("expected status bar to show the status message, got %q", bar)
	}
}
