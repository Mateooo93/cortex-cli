package ui

import (
	"strings"
	"testing"
)

func TestBuildModelPickerEntries_IncludesCodex(t *testing.T) {
	entries := buildModelPickerEntries()
	if len(entries) == 0 {
		t.Fatal("buildModelPickerEntries returned no entries")
	}
	hasCodex := false
	for _, e := range entries {
		if e.ProviderName == "codex" {
			hasCodex = true
			break
		}
	}
	if !hasCodex {
		t.Error("picker missing codex provider entries")
	}
}

func TestBuildModelPickerEntries_ProviderLabelShowsAuthKind(t *testing.T) {
	entries := buildModelPickerEntries()
	wantSubstr := map[string]string{
		"codex":     "OAuth (subscription)",
		"openai":    "API key",
		"ollama":    "no key",
		"bedrock":   "env var",
	}
	for provider, want := range wantSubstr {
		found := false
		for _, e := range entries {
			if e.ProviderName == provider && strings.Contains(e.ProviderLabel, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("provider %q: no entry has label containing %q", provider, want)
		}
	}
}

func TestFilterModelPickerEntries(t *testing.T) {
	entries := buildModelPickerEntries()
	if len(entries) == 0 {
		t.Fatal("no entries")
	}
	// Filter for "ChatGPT" should return only codex entries.
	filtered := filterModelPickerEntries(entries, "ChatGPT")
	for _, e := range filtered {
		if !strings.Contains(e.DisplayName, "ChatGPT") &&
			!strings.Contains(e.ProviderLabel, "ChatGPT") {
			t.Errorf("filter returned non-ChatGPT entry: %+v", e)
		}
	}
	// Empty query returns all.
	all := filterModelPickerEntries(entries, "")
	if len(all) != len(entries) {
		t.Errorf("empty filter changed result: %d vs %d", len(all), len(entries))
	}
}

func TestModelPicker_OpenCloseQuery(t *testing.T) {
	p := NewModelPicker()
	if p.IsVisible() {
		t.Error("fresh picker shouldn't be visible")
	}
	p.Open(nil)
	if !p.IsVisible() {
		t.Error("Open should make picker visible")
	}
	p.SetQuery("gpt")
	if p.Query() != "gpt" {
		t.Errorf("Query() = %q, want \"gpt\"", p.Query())
	}
	p.Close()
	if p.IsVisible() {
		t.Error("Close should hide picker")
	}
}

func TestModelPicker_Navigation(t *testing.T) {
	p := NewModelPicker()
	p.Open(nil)
	if len(p.entries) < 3 {
		t.Skip("need at least 3 entries for navigation test")
	}
	if p.Selected() == "" {
		t.Error("expected initial selection")
	}
	first := p.Selected()
	p.MoveDown()
	if p.Selected() == first {
		t.Error("MoveDown didn't move selection")
	}
	p.MoveUp()
	if p.Selected() != first {
		t.Error("MoveUp didn't return to first")
	}
}
