package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestAutolinkBareURLs(t *testing.T) {
	in := "see https://example.com/docs for more"
	got := autolinkBareURLs(in)
	want := "see [https://example.com/docs](https://example.com/docs) for more"
	if got != want {
		t.Fatalf("autolinkBareURLs() = %q, want %q", got, want)
	}
}

func TestChatURLAtBareURL(t *testing.T) {
	line := "docs at https://example.com/path and more"
	if got := chatURLAt(line, 8); got != "https://example.com/path" {
		t.Fatalf("chatURLAt() = %q, want URL at column 8", got)
	}
	if got := chatURLAt(line, 0); got != "" {
		t.Fatalf("chatURLAt() = %q, want empty outside URL", got)
	}
}

func TestChatURLAtOSC8Hyperlink(t *testing.T) {
	line := ansi.SetHyperlink("https://example.com") + "click here" + ansi.ResetHyperlink()
	if got := chatURLAt(line, 2); got != "https://example.com" {
		t.Fatalf("chatURLAt() = %q, want OSC-8 hyperlink URL", got)
	}
}

func TestHyperlinkAtColResets(t *testing.T) {
	line := ansi.SetHyperlink("https://a.test") + "link" + ansi.ResetHyperlink() + "plain"
	if got := hyperlinkAtCol(line, 6); got != "" {
		t.Fatalf("hyperlinkAtCol after reset = %q, want empty", got)
	}
}

func TestAutolinkBareURLsTrimsTrailingPunctuation(t *testing.T) {
	got := autolinkBareURLs("visit https://example.com.")
	if !strings.Contains(got, "(https://example.com)") {
		t.Fatalf("expected trimmed URL in markdown link, got %q", got)
	}
	if strings.Contains(got, "example.com.)") {
		t.Fatalf("trailing period should stay outside link, got %q", got)
	}
}