package ui

import (
	"strings"
	"testing"
)

func TestRenderInputKeybindHint_ShowsEnterTabEsc(t *testing.T) {
	hint := stripANSI(renderInputKeybindHint(80))
	for _, key := range []string{"Enter", "Tab", "Esc"} {
		if !strings.Contains(hint, key) {
			t.Fatalf("expected %q in hint, got %q", key, hint)
		}
	}
}