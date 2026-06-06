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

// TestRenderStatusBarSlimDefault verifies the new slim footer
// shows: connection state, model tag, ctx percentage, elapsed
// time, and the F1/F2/F3 tab bar — all on a single line. The
// user complained the old 2-line design was cluttered; this
// test pins the new shape.
func TestRenderStatusBarSlimDefault(t *testing.T) {
	s := NewStyles(true)
	info := StatusBarInfo{
		ModelName:   "GPT-5.5 (ChatGPT)",
		ProviderTag: "codex",
		InputTokens: 12_345,
		ContextMax:  200_000,
		Elapsed:     2*time.Minute + 13*time.Second,
	}
	bar := renderStatusBar(120, true, false, StatusMessage{}, s, info)
	lines := strings.Split(bar, "\n")
	if len(lines) != 1 {
		t.Fatalf("slim status bar should be 1 line, got %d: %q", len(lines), bar)
	}
	for _, want := range []string{"connected", "GPT-5.5", "codex", "12k", "200k", "2:13", "F1", "F2", "F3"} {
		if !strings.Contains(bar, want) {
			t.Errorf("status bar missing %q, got %q", want, bar)
		}
	}
}

// TestRenderStatusBarTwoLinesWhenMsgActive verifies the footer
// grows to 2 lines when a transient message is active so the
// user can see the message without losing the readouts.
func TestRenderStatusBarTwoLinesWhenMsgActive(t *testing.T) {
	s := NewStyles(true)
	info := StatusBarInfo{
		ModelName:   "GPT-5.5",
		InputTokens: 1000,
		ContextMax:  200_000,
	}
	bar := renderStatusBar(120, true, false, StatusMessage{Text: "out of API credits", Kind: StatusMsgError}, s, info)
	lines := strings.Split(bar, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected status bar to expand to 2+ lines when msg active, got %d: %q", len(lines), bar)
	}
	if !strings.Contains(bar, "out of API credits") {
		t.Errorf("expected status bar to include the error message, got %q", bar)
	}
}

// TestRenderStatusBarOmitEmpty verifies the footer gracefully
// drops empty fields (no model selected, no context, etc.) so
// the line doesn't become a wall of "—" placeholders.
func TestRenderStatusBarOmitEmpty(t *testing.T) {
	s := NewStyles(true)
	bar := renderStatusBar(120, true, false, StatusMessage{}, s, StatusBarInfo{})
	// No model, no ctx, no elapsed — just the connection
	// status and the F1/F2/F3 tab bar. Should NOT contain
	// "ctx" or "⏱" placeholders.
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
	bar := renderStatusBar(120, true, false, StatusMessage{}, s, StatusBarInfo{
		ModelName:  "GPT-5.5",
		QueuedMsgs: 1,
	})
	if !strings.Contains(bar, "1 queued") {
		t.Errorf("expected status bar to show '1 queued', got %q", bar)
	}
}
