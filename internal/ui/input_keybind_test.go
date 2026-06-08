package ui

import (
	"strings"
	"testing"
)

func TestRenderInputKeybindHint_ShowsEnterTabEsc(t *testing.T) {
	raw := renderInputKeybindHint(80)
	hint := stripANSI(raw)
	for _, key := range []string{"Enter", "Tab", "Esc", "send", "queue", "cancel"} {
		if !strings.Contains(hint, key) {
			t.Fatalf("expected %q in hint, got %q", key, hint)
		}
	}
	if !strings.Contains(hint, "│") {
		t.Fatalf("expected │ separators between keybinds, got %q", hint)
	}
	if strings.Contains(raw, "48;") {
		t.Fatalf("keybind hint should not use background badges, got %q", hint)
	}
}