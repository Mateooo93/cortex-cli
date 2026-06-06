package ui

import (
	"strings"
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// TestAvailableModels_AllPrefixed asserts every catalogue entry's Spec
// starts with its declared Provider name plus a slash. The settings UI
// dispatches Spec verbatim to session.set_model; if a row leaks a bare
// name through, ParseModel rejects it and the model picker breaks.
func TestAvailableModels_AllPrefixed(t *testing.T) {
	for _, m := range AvailableModels {
		wantPrefix := m.Provider + "/"
		if !strings.HasPrefix(m.Spec, wantPrefix) {
			t.Errorf("model %q has Provider=%q but Spec doesn't start with %q", m.DisplayName, m.Provider, wantPrefix)
		}
	}
}

// TestModelsForProvider_GroupsCorrectly asserts the filter returns models
// whose Provider matches AND covers every provider in AvailableProviders.
func TestModelsForProvider_GroupsCorrectly(t *testing.T) {
	for _, p := range AvailableProviders {
		models := ModelsForProvider(p.Name)
		if len(models) == 0 {
			t.Errorf("provider %q has no models in AvailableModels", p.Name)
		}
		for _, m := range models {
			if m.Provider != p.Name {
				t.Errorf("ModelsForProvider(%q) returned a model whose Provider=%q", p.Name, m.Provider)
			}
		}
	}
}

// TestProviderOf covers the prefix extraction including the OpenRouter
// nested-route case (we want the provider WE talk to, not the upstream).
func TestProviderOf(t *testing.T) {
	cases := []struct {
		spec string
		want string
	}{
		{"anthropic/claude-opus-4-8", "anthropic"},
		{"openrouter/anthropic/claude-opus-4-8", "openrouter"},
		{"openai/o3", "openai"},
		{"minimax/MiniMax-M2.7", "minimax"},
		{"claude-sonnet-4-6", ""},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.spec, func(t *testing.T) {
			if got := ProviderOf(c.spec); got != c.want {
				t.Errorf("ProviderOf(%q) = %q, want %q", c.spec, got, c.want)
			}
		})
	}
}

// TestLocateActiveModel covers the cursor-positioning logic the Settings
// tab uses when it opens with a model already selected.
func TestModelsForProviderFromConfig_NormalizesOpenGatewayMiniMax(t *testing.T) {
	cfg := cortexconfig.Default()
	cfg.Models["opengateway"] = cortexconfig.ModelConfig{
		Provider: "opengateway",
		Model:    "minimax-m3",
		BaseURL:  "https://opengateway.gitlawb.com/v1",
	}
	models := ModelsForProviderFromConfig("opengateway", cfg)
	if len(models) == 0 {
		t.Fatal("expected opengateway models")
	}
	if models[0].Spec != "opengateway/minimax/minimax-m3" {
		t.Fatalf("first spec = %q, want opengateway/minimax/minimax-m3", models[0].Spec)
	}
	if models[0].DisplayName != "minimax/minimax-m3" {
		t.Fatalf("first display = %q, want minimax/minimax-m3", models[0].DisplayName)
	}
}

func TestLocateActiveModelFromConfig_NormalizesOldOpenGatewaySpec(t *testing.T) {
	cfg := cortexconfig.Default()
	cfg.DefaultModel = "opengateway"
	cfg.Models["opengateway"] = cortexconfig.ModelConfig{
		Provider: "opengateway",
		Model:    "minimax-m3",
		BaseURL:  "https://opengateway.gitlawb.com/v1",
	}
	pi, mi := locateActiveModelFromConfig("opengateway", cfg)
	providers := ProvidersFromConfig(cfg)
	if providers[pi].Name != "opengateway" {
		t.Fatalf("provider = %q, want opengateway", providers[pi].Name)
	}
	models := ModelsForProviderFromConfig("opengateway", cfg)
	if models[mi].Spec != "opengateway/minimax/minimax-m3" {
		t.Fatalf("model spec = %q, want opengateway/minimax/minimax-m3", models[mi].Spec)
	}
}
