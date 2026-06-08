package ui

import "testing"

func TestRenderStreamingAssistant_ShowsCursorOnLastLine(t *testing.T) {
	got := renderStreamingAssistant("hello\nworld", 40, true, true)
	if got == "" {
		t.Fatal("expected rendered streaming preview")
	}
	if !containsANSI(got) {
		t.Fatalf("expected styled output, got %q", got)
	}
	last := got
	if i := len(last) - 1; i >= 0 && last[i] == '\n' {
		last = last[:i]
	}
	if !containsRune(last, '▌') {
		t.Fatalf("expected streaming cursor on last line, got %q", got)
	}
}

func TestRenderStreamingAssistant_HidesCursorWhenOff(t *testing.T) {
	got := renderStreamingAssistant("hello", 40, true, false)
	if containsRune(got, '▌') {
		t.Fatalf("did not expect cursor, got %q", got)
	}
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

func containsANSI(s string) bool {
	return len(s) > 0 && (containsRune(s, '\x1b') || containsRune(s, 0x1b))
}