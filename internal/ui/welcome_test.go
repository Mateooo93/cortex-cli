package ui

import (
	"strings"
	"testing"
)

func TestBuildWelcomeLinesCentersBanner(t *testing.T) {
	s := NewStyles(true)
	lines := buildWelcomeLines(100, s)
	if len(lines) < 6 {
		t.Fatal("expected full banner lines")
	}
	first := lines[0]
	if !strings.HasPrefix(first, " ") {
		t.Fatalf("banner line should be horizontally centered, got %q", first)
	}
	if !strings.Contains(first, "██████╗") {
		t.Fatal("expected ASCII banner art")
	}
}

func TestWelcomeViewportLinesCentersVertically(t *testing.T) {
	s := NewStyles(true)
	viewport := welcomeViewportLines(100, 30, s)
	if len(viewport) != 30 {
		t.Fatalf("viewport height = %d, want 30", len(viewport))
	}
	joined := strings.Join(viewport, "\n")
	if !strings.Contains(joined, "██████╗") {
		t.Fatal("expected banner in viewport")
	}
	// Banner should not be flush against the top in a tall viewport.
	if strings.HasPrefix(strings.TrimLeft(viewport[0], " "), "█") {
		t.Fatal("banner should be vertically centered, not top-aligned")
	}
}

func TestCortexBannerFollowsPrimaryColor(t *testing.T) {
	oldPrimary := primaryHex
	t.Cleanup(func() {
		primaryHex = oldPrimary
	})

	primaryHex = "#8B5CF6"
	banner := strings.Join(renderCortexBanner(), "\n")
	// True-color SGR for violet #8B5CF6 → rgb(139, 92, 246)
	if !strings.Contains(banner, "139;92;246") {
		t.Fatalf("expected banner to use theme primary violet, got %q", stripANSI(banner))
	}
}

func TestCortexBannerTopLayersPreserveWarmHue(t *testing.T) {
	oldPrimary := primaryHex
	t.Cleanup(func() { primaryHex = oldPrimary })

	tests := []struct {
		hex    string
		substr string // true-color SGR fragment expected on the top banner row
	}{
		// Amber top row should stay orange-gold, not Lab-shift to grey/green.
		{"#F59E0B", "249;170;"},
		// Rose top row should stay red-pink, not cyan-grey.
		{"#F43F5E", "248;84;"},
	}
	for _, tc := range tests {
		primaryHex = tc.hex
		top := renderCortexBanner()[0]
		if !strings.Contains(top, tc.substr) {
			t.Fatalf("primary %s: top banner row should contain %q, got %q", tc.hex, tc.substr, stripANSI(top))
		}
	}
}

func TestRenderWelcomeInlineNarrowWidthUsesCompactBanner(t *testing.T) {
	s := NewStyles(true)
	lines := buildWelcomeLines(40, s)
	if strings.Contains(strings.Join(lines, "\n"), "██████╗") {
		t.Fatal("narrow welcome should use compact banner, not full ASCII art")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "CORTEX") {
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