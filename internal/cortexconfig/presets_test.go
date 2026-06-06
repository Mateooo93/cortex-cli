package cortexconfig

import (
	"strings"
	"testing"
)

// TestBuiltinProviderPresets_Coverage checks that every popular
// provider a new user might expect is in the preset list, with
// at minimum a name and a base URL.
func TestBuiltinProviderPresets_Coverage(t *testing.T) {
	want := []string{
		// OAuth (subscriptions) — no API key
		"codex", "claude-sub", "copilot",
		// API-key providers (paid)
		"openai", "anthropic", "gemini", "xai", "deepseek",
		"mistral", "groq", "cohere", "perplexity",
		// Aggregators
		"openrouter", "opengateway", "minimax", "mimo", "bedrock",
		// Local / keyless
		"cortex", "ollama", "lmstudio", "vllm",
	}
	have := map[string]bool{}
	for _, p := range BuiltinProviderPresets {
		have[p.Name] = true
		if p.BaseURL == "" {
			t.Errorf("preset %q has empty BaseURL", p.Name)
		}
		if p.DisplayName == "" {
			t.Errorf("preset %q has empty DisplayName", p.Name)
		}
		if p.DefaultModel == "" {
			t.Errorf("preset %q has empty DefaultModel", p.Name)
		}
	}
	for _, name := range want {
		if !have[name] {
			t.Errorf("missing built-in provider preset: %q", name)
		}
	}
}

// TestBuiltinProviderPresets_NoDuplicateNames catches accidental
// duplicates in the preset list (which would silently shadow earlier
// entries when the Settings tab resolves a provider name).
func TestBuiltinProviderPresets_NoDuplicateNames(t *testing.T) {
	seen := map[string]int{}
	for i, p := range BuiltinProviderPresets {
		if prev, ok := seen[p.Name]; ok {
			t.Errorf("duplicate preset name %q at index %d (first seen at %d)", p.Name, i, prev)
		}
		seen[p.Name] = i
	}
}

// TestAuthKind_SubscriptionProvidersHaveNoKey verifies that every
// OAuth-style preset (subscription) has NeedsAPIKey=false and
// AuthKind="oauth". This is the most important invariant in the
// preset list: a user who already pays for ChatGPT Plus or Claude
// Pro must NEVER be asked for an API key.
func TestAuthKind_SubscriptionProvidersHaveNoKey(t *testing.T) {
	oauth := []string{"codex", "claude-sub", "copilot"}
	for _, name := range oauth {
		p, ok := presetForProvider(name)
		if !ok {
			t.Errorf("oauth preset %q missing from BuiltinProviderPresets", name)
			continue
		}
		if p.NeedsAPIKey {
			t.Errorf("oauth preset %q has NeedsAPIKey=true; should be false (subscription auth)", name)
		}
		if p.AuthKind != "oauth" {
			t.Errorf("oauth preset %q has AuthKind=%q; want \"oauth\"", name, p.AuthKind)
		}
		if p.HelpURL == "" {
			t.Errorf("oauth preset %q has empty HelpURL; should point at the sign-in page", name)
		}
	}
}

// TestAuthKind_LocalProvidersHaveNoKey checks the local-server
// presets (Cortex, Ollama, LM Studio, vLLM) all have NeedsAPIKey=false
// and AuthKind="none".
func TestAuthKind_LocalProvidersHaveNoKey(t *testing.T) {
	local := []string{"cortex", "ollama", "lmstudio", "vllm"}
	for _, name := range local {
		p, ok := presetForProvider(name)
		if !ok {
			t.Errorf("local preset %q missing", name)
			continue
		}
		if p.NeedsAPIKey {
			t.Errorf("local preset %q has NeedsAPIKey=true", name)
		}
		if p.AuthKind != "none" {
			t.Errorf("local preset %q has AuthKind=%q; want \"none\"", name, p.AuthKind)
		}
	}
}

// TestAuthKind_APIStyleNeedsKey checks that every API-key-style
// preset has NeedsAPIKey=true. If any of these slip through as
// NeedsAPIKey=false, the Settings tab will silently skip the API
// key input row and the provider will fail at chat time.
func TestAuthKind_APIStyleNeedsKey(t *testing.T) {
	want := []string{
		"openai", "anthropic", "gemini", "xai", "deepseek",
		"mistral", "groq", "cohere", "perplexity",
		"openrouter", "opengateway", "minimax", "mimo",
	}
	for _, name := range want {
		p, ok := presetForProvider(name)
		if !ok {
			t.Errorf("api-key preset %q missing", name)
			continue
		}
		if !p.NeedsAPIKey {
			t.Errorf("api-key preset %q has NeedsAPIKey=false; should be true", name)
		}
		if p.APIKeyEnvVar == "" {
			t.Errorf("api-key preset %q has empty APIKeyEnvVar", name)
		}
	}
}

// TestAuthKind_BaseURLsAreHTTPSOrLocal checks that no cloud provider
// is using plain HTTP (which would leak the API key).
func TestAuthKind_BaseURLsAreHTTPSOrLocal(t *testing.T) {
	cloud := []string{
		"openai", "anthropic", "gemini", "xai", "deepseek",
		"mistral", "groq", "cohere", "perplexity",
		"openrouter", "opengateway", "minimax", "mimo",
		"codex", "claude-sub", "copilot", "bedrock",
	}
	for _, name := range cloud {
		p, ok := presetForProvider(name)
		if !ok {
			continue
		}
		if !strings.HasPrefix(p.BaseURL, "https://") {
			t.Errorf("cloud preset %q has non-HTTPS base URL: %q", name, p.BaseURL)
		}
	}
}

// TestProviderAuthKind_UnknownReturnsApikey is the safe-default
// behaviour: an unknown provider name is treated as needing an
// API key (not OAuth, not "none"). Better to ask for a key the
// user doesn't have than to silently no-op.
func TestProviderAuthKind_UnknownReturnsApikey(t *testing.T) {
	got := ProviderAuthKind("totally-unknown-provider-xyz")
	if got != "apikey" {
		t.Errorf("ProviderAuthKind(unknown) = %q, want \"apikey\"", got)
	}
}

// TestProviderAuthKind_CodexIsOAuth explicitly pins the most-asked
// invariant: codex is OAuth, not API-key. If this test fails,
// users with a ChatGPT Plus subscription will be asked for an
// OpenAI API key they don't have.
func TestProviderAuthKind_CodexIsOAuth(t *testing.T) {
	if got := ProviderAuthKind("codex"); got != "oauth" {
		t.Errorf("ProviderAuthKind(\"codex\") = %q, want \"oauth\"", got)
	}
	if ProviderNeedsAPIKey("codex") {
		t.Errorf("ProviderNeedsAPIKey(\"codex\") = true, want false (subscription auth)")
	}
}

// TestEnsureProviderPresets_AddsNewOnes verifies that a config
// created before the new presets were added gets the new entries
// the next time it loads.
func TestEnsureProviderPresets_AddsNewOnes(t *testing.T) {
	// Start with an "old" config that only knows about the legacy
	// 9 providers.
	c := &Config{
		Models: map[string]ModelConfig{
			"openai": {Provider: "openai", Model: "gpt-4o"},
		},
	}
	c.EnsureProviderPresets()
	for _, want := range []string{"codex", "claude-sub", "gemini", "deepseek", "xai", "groq", "ollama", "cortex"} {
		if _, ok := c.Models[want]; !ok {
			t.Errorf("EnsureProviderPresets didn't add %q", want)
		}
	}
}
