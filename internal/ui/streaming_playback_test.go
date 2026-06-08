package ui

import (
	"strings"
	"testing"
)

func TestReleaseStreamPlayback_AdaptiveSizing(t *testing.T) {
	pending := "abcdefghijklmnopqrstuvwxyz"
	got := releaseStreamPlayback(&pending)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 chars released, got %q", got)
	}
	if pending == "" {
		t.Fatal("expected remainder in pending buffer")
	}

	pending = strings.Repeat("x", 500)
	got = releaseStreamPlayback(&pending)
	if len(got) < 24 {
		t.Fatalf("expected large release for backlog, got %d chars", len(got))
	}
}