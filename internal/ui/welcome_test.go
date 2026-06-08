package ui

import (
	"strings"
	"testing"
)

func TestRenderWelcomeInlineNarrowWidthUsesCompactBanner(t *testing.T) {
	s := NewStyles(true)
	out := renderWelcomeInline(40, 20, s)
	if strings.Contains(out, "██████╗") {
		t.Fatal("narrow welcome should use compact banner, not full ASCII art")
	}
	if !strings.Contains(out, "CORTEX") {
		t.Fatal("expected compact CORTEX banner")
	}
}

func TestUpdateWidthDoesNotResetTo80OnNarrowTerminal(t *testing.T) {
	r := NewMarkdownRenderer(80, true, NewStyles(true).CodeBoxBorderStyle)
	r.UpdateWidth(12)
	if r.width != 12 {
		t.Fatalf("UpdateWidth(12) set width=%d, want 12 (not 80)", r.width)
	}
}

func TestRenderWelcomeInlineWideWidthKeepsBannerLines(t *testing.T) {
	s := NewStyles(true)
	out := renderWelcomeInline(80, 24, s)
	if !strings.Contains(out, "██████╗") {
		t.Fatal("wide welcome should render full ASCII banner")
	}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if w := len([]rune(line)); w > 120 {
			t.Fatalf("welcome line unexpectedly long (%d chars): %q", w, line)
		}
	}
}