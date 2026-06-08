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

func TestFormatTokenCountShort(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{512, "512"},
		{1500, "1.5k"},
		{12_345, "12k"},
		{1_500_000, "1.5M"},
	}
	for _, tt := range tests {
		got := formatTokenCountShort(tt.n)
		if got != tt.want {
			t.Errorf("formatTokenCountShort(%d) = %q, want %q", tt.n, got, tt.want)
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

// TestRenderStatusBarSlimDefault verifies the slim footer shows
// model tag, ctx percentage, and elapsed time on a single line.
func TestRenderStatusBarSlimDefault(t *testing.T) {
	s := NewStyles(true)
	info := StatusBarInfo{
		ModelName:   "GPT-5.5 (ChatGPT)",
		ProviderTag: "codex",
		InputTokens: 12_345,
		ContextMax:  200_000,
		Elapsed:     2*time.Minute + 13*time.Second,
	}
	bar := renderStatusBar(120, StatusMessage{}, s, info)
	lines := strings.Split(bar, "\n")
	if len(lines) != 1 {
		t.Fatalf("slim status bar should be 1 line, got %d: %q", len(lines), bar)
	}
	for _, want := range []string{"GPT-5.5", "codex", "12k", "200k", "2:13"} {
		if !strings.Contains(bar, want) {
			t.Errorf("status bar missing %q, got %q", want, bar)
		}
	}
	for _, unwanted := range []string{"connected", "disconnected", "reconnecting"} {
		if strings.Contains(bar, unwanted) {
			t.Errorf("status bar should not show connection state %q, got %q", unwanted, bar)
		}
	}
	// F1-F4 should NOT be in the slim footer anymore.
	for _, unwanted := range []string{" F1 ", " F2 ", " F3 ", " F4 "} {
		if strings.Contains(bar, unwanted) {
			t.Errorf("slim footer should not contain %q (moved to tab bar), got %q", unwanted, bar)
		}
	}
}

// TestRenderStatusBarCollapsesToOneLineWhenMsgActive pins
// the user-reported bug fix: the status bar used to grow
// to 2 lines when a transient message was active, which
// overlapped the bottom row of the chat viewport and
// made the bottom of the conversation appear to
// "disappear". The fix collapses the status bar to a
// single line (the message REPLACES the slim footer)
// so the layout never overflows its reserved 1 row.
// The user-reported bug: "when i scroll up the bottom
// of the chat starts disappearing and at some point
// half of the conversation section is invisible".
func TestRenderStatusBarCollapsesToOneLineWhenMsgActive(t *testing.T) {
	s := NewStyles(true)
	info := StatusBarInfo{
		ModelName:   "GPT-5.5",
		InputTokens: 1000,
		ContextMax:  200_000,
	}
	bar := renderStatusBar(120, StatusMessage{Text: "out of API credits", Kind: StatusMsgError}, s, info)
	// Status bar must be EXACTLY 1 line tall when a
	// message is active — never 2 — so the layout
	// (which reserves 1 row for the status bar) never
	// overflows into the chat viewport.
	lines := strings.Split(strings.TrimRight(bar, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected status bar to be exactly 1 line when msg active (was 2, overlapping chat), got %d: %q", len(lines), bar)
	}
	if !strings.Contains(bar, "out of API credits") {
		t.Errorf("expected status bar to include the error message, got %q", bar)
	}
}

// TestRenderStatusBarSpinnerOneLine pins the same
// behaviour for the spinner case: the /update flow
// runs a braille spinner next to the message. The
// status bar must still be exactly 1 line tall so
// the spinner doesn't push into the chat viewport.
func TestRenderStatusBarSpinnerOneLine(t *testing.T) {
	s := NewStyles(true)
	bar := renderStatusBar(120, StatusMessage{Text: "Downloading…", Kind: StatusMsgInfo, Spinner: 3}, s, StatusBarInfo{})
	lines := strings.Split(strings.TrimRight(bar, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1-line status bar with spinner, got %d: %q", len(lines), bar)
	}
	if !strings.Contains(bar, "Downloading…") {
		t.Errorf("expected message in bar, got %q", bar)
	}
}

// TestRenderStatusBarOmitEmpty verifies the footer gracefully
// drops empty fields (no model selected, no context, etc.) so
// the line doesn't become a wall of "—" placeholders.
func TestRenderStatusBarOmitEmpty(t *testing.T) {
	s := NewStyles(true)
	bar := renderStatusBar(120, StatusMessage{}, s, StatusBarInfo{})
	// No model, no ctx, no elapsed — should NOT contain
	// "ctx" or "⏱" placeholders.
	if strings.Contains(bar, "connected") {
		t.Errorf("expected status bar to omit connection state, got %q", bar)
	}
	if strings.Contains(bar, "ctx ") {
		t.Errorf("expected status bar to omit 'ctx' when no tokens, got %q", bar)
	}
	if strings.Contains(bar, "⏱") {
		t.Errorf("expected status bar to omit elapsed when zero, got %q", bar)
	}
}

// TestRenderStatusBarShowsQueuedCount verifies a queued message
// shows up as "1 queued" in the footer so the user knows their
// Tab press landed.
func TestRenderStatusBarShowsQueuedCount(t *testing.T) {
	s := NewStyles(true)
	bar := renderStatusBar(120, StatusMessage{}, s, StatusBarInfo{
		ModelName:  "GPT-5.5",
		QueuedMsgs: 1,
	})
	if !strings.Contains(bar, "1 queued") {
		t.Errorf("expected status bar to show '1 queued', got %q", bar)
	}
}
