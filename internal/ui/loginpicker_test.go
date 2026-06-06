package ui

import (
	"testing"
)

// TestLoginPicker_BuildsOAuthOnly verifies the /login picker
// contains exactly the three OAuth providers (codex, claude-sub,
// copilot). API-key providers (openai, anthropic, etc.) must NOT
// be in the list — those are configured through the right-panel
// key input form, not through a subscription sign-in flow.
func TestLoginPicker_BuildsOAuthOnly(t *testing.T) {
	entries := buildLoginPickerEntries()
	if len(entries) != 3 {
		t.Errorf("entries = %d, want 3 (codex, claude-sub, copilot)", len(entries))
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Provider] = true
		if e.AuthMethod == "" {
			t.Errorf("%s: AuthMethod empty", e.Provider)
		}
	}
	for _, want := range []string{"codex", "claude-sub", "copilot"} {
		if !seen[want] {
			t.Errorf("missing provider %q in picker", want)
		}
	}
}

// TestLoginPicker_FilterByQuery covers the type-to-filter UX:
// typing "codex" leaves only the codex row visible.
func TestLoginPicker_FilterByQuery(t *testing.T) {
	all := buildLoginPickerEntries()
	got := filterLoginPickerEntries(all, "codex")
	if len(got) != 1 || got[0].Provider != "codex" {
		t.Errorf("filter 'codex' = %v, want [codex]", providersOf(got))
	}
	// Typing "device" should match the codex row because the
	// help text mentions the device-code fallback. (Used to
	// route the user to the device-code flow on Enter.)
	got2 := filterLoginPickerEntries(all, "device")
	if len(got2) != 1 || got2[0].Provider != "codex" {
		t.Errorf("filter 'device' = %v, want [codex]", providersOf(got2))
	}
	// Empty query returns all.
	if got3 := filterLoginPickerEntries(all, ""); len(got3) != 3 {
		t.Errorf("empty filter = %d, want 3", len(got3))
	}
}

// TestLoginPicker_DeviceFlagFromQuery is the headline behavior:
// typing "codex --device" then pressing Enter must report
// (provider="codex", wantDevice=true) so the device-code flow
// runs instead of the browser flow. The bug it's catching: if
// the user is on a remote machine (no localhost browser) the
// browser flow always fails, so we need a one-keystroke way to
// switch flows.
func TestLoginPicker_DeviceFlagFromQuery(t *testing.T) {
	p := NewLoginPicker()
	p.Open()
	p.SetQuery("codex --device")
	provider, wantDevice := p.Selected()
	if provider != "codex" {
		t.Errorf("provider = %q, want codex", provider)
	}
	if !wantDevice {
		t.Errorf("wantDevice = false, want true (query contained 'device')")
	}
}

func providersOf(es []LoginPickerEntry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Provider
	}
	return out
}
