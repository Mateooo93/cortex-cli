package ui

import (
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// TestModelPicker_CustomProvider_SurfacesInEntries verifies that
// a provider the user added in Settings (so it's in cfg.Models
// but NOT in the curated AvailableModels catalogue) shows up in
// the /model picker. Before the fix, buildModelPickerEntries
// ignored the cortexcfg argument, so freshly-added providers
// were invisible.
func TestModelPicker_CustomProvider_SurfacesInEntries(t *testing.T) {
	cfg := &cortexconfig.Config{
		Models: map[string]cortexconfig.ModelConfig{
			"my-local": {
				Provider: "my-local",
				Model:    "qwen2.5-coder-32b",
				BaseURL:  "https://api.example.com/v1",
			},
		},
	}
	cfg.EnsureProviderPresets()
	entries := buildModelPickerEntries(cfg)
	var found *ModelPickerEntry
	for i := range entries {
		if entries[i].ProviderName == "my-local" {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("custom provider 'my-local' not in picker entries (got %d entries)", len(entries))
	}
	if found.Spec != "my-local/qwen2.5-coder-32b" {
		t.Errorf("expected spec 'my-local/qwen2.5-coder-32b', got %q", found.Spec)
	}
}

// TestModelPicker_Open_IncludesCustomProvider covers the same
// scenario from the Open() entry-point: the picker's entries
// must include freshly-added providers as soon as the user
// hits /model.
func TestModelPicker_Open_IncludesCustomProvider(t *testing.T) {
	cfg := &cortexconfig.Config{
		Models: map[string]cortexconfig.ModelConfig{
			"my-local": {
				Provider: "my-local",
				Model:    "qwen2.5-coder-32b",
			},
		},
	}
	cfg.EnsureProviderPresets()
	var p ModelPicker
	p.Open(cfg)
	for _, e := range p.entries {
		if e.ProviderName == "my-local" {
			return
		}
	}
	t.Errorf("picker not populated with custom provider; got %d entries", len(p.entries))
}
