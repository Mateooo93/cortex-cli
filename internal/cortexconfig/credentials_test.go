package cortexconfig

import "testing"

func TestApplyBuiltinZenFallback_SwitchesWhenNoKey(t *testing.T) {
	if !testingKeyAvailable() {
		t.Skip("embedded zen key not available")
	}
	cfg := Default()
	cfg.DefaultModel = "openai"
	cfg.Models["openai"] = ModelConfig{
		Provider: "openai",
		Model:    "gpt-5.5",
		BaseURL:  "https://api.openai.com/v1",
	}
	cfg.ApplyBuiltinZenFallback()
	if !cfg.HasUsableCredentials() {
		t.Fatal("expected usable credentials after zen fallback")
	}
	if cfg.DefaultModel != BuiltinZenProvider && cfg.DefaultModel != ModelSpec(BuiltinZenProvider, BuiltinZenModel) {
		t.Fatalf("DefaultModel = %q, want opencode-zen", cfg.DefaultModel)
	}
}

func TestResolveProviderAPIKey_UsesEmbeddedZen(t *testing.T) {
	if !testingKeyAvailable() {
		t.Skip("embedded zen key not available")
	}
	cfg := Default()
	mc := cfg.Models[BuiltinZenProvider]
	key := ResolveProviderAPIKey(cfg, BuiltinZenProvider, &mc)
	if key == "" {
		t.Fatal("expected embedded zen key")
	}
}

func TestResolveProviderAPIKey_EnvWins(t *testing.T) {
	t.Setenv("OPENCODE_ZEN_API_KEY", "sk-test-env-key")
	cfg := Default()
	mc := cfg.Models[BuiltinZenProvider]
	key := ResolveProviderAPIKey(cfg, BuiltinZenProvider, &mc)
	if key != "sk-test-env-key" {
		t.Fatalf("key = %q, want env override", key)
	}
}

func testingKeyAvailable() bool {
	cfg := Default()
	mc := cfg.Models[BuiltinZenProvider]
	return ResolveProviderAPIKey(cfg, BuiltinZenProvider, &mc) != ""
}

func TestDefaultUsesOpenCodeZen(t *testing.T) {
	cfg := Default()
	if cfg.DefaultModel != BuiltinZenProvider {
		t.Fatalf("DefaultModel = %q", cfg.DefaultModel)
	}
	mc, ok := cfg.Models[BuiltinZenProvider]
	if !ok {
		t.Fatal("missing opencode-zen model entry")
	}
	if mc.Model != BuiltinZenModel {
		t.Fatalf("model = %q", mc.Model)
	}
	if mc.BaseURL != "https://opencode.ai/zen/v1" {
		t.Fatalf("baseURL = %q", mc.BaseURL)
	}
}
