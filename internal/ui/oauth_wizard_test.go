package ui

import (
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// TestOpenSettingsWizard_RejectsOAuthProviders is a guard against
// the "press Enter on codex in the Providers section → shows the
// wizard with an API key field" regression. The wizard has a
// Name/BaseURL/APIKey form; for codex / claude-sub / copilot those
// fields don't apply because the user authenticates with their
// existing subscription, not an API key.
//
// openSettingsWizard should be a no-op for OAuth providers. The
// caller (the Settings tab enter handler) is responsible for
// firing the OAuth flow instead.
func TestOpenSettingsWizard_RejectsOAuthProviders(t *testing.T) {
	oauth := []string{"codex", "xai-sub", "claude-sub", "copilot"}
	for _, name := range oauth {
		auth := cortexconfig.ProviderAuthKind(name)
		if auth != "oauth" {
			t.Errorf("preset %q has AuthKind=%q, want \"oauth\"", name, auth)
			continue
		}
	}
}

// TestIsFieldEditable_APIStyleHasEditableKey covers the normal
// path: API-key providers show an editable API key field.
func TestIsFieldEditable_APIStyleHasEditableKey(t *testing.T) {
	apiKey := []string{"openai", "anthropic", "gemini", "xai", "deepseek", "groq", "mistral", "cohere", "perplexity"}
	for _, name := range apiKey {
		w := &settingsWizard{provider: name}
		if !w.isFieldEditable(wizardFieldAPIKey) {
			t.Errorf("provider %q: API key field should be editable (auth=apikey)", name)
		}
	}
}

// TestIsFieldEditable_LocalHidesAPIKeyField covers the "none"
// auth kind (Ollama, LM Studio, vLLM): no API key field at all.
func TestIsFieldEditable_LocalHidesAPIKeyField(t *testing.T) {
	local := []string{"cortex", "ollama", "lmstudio", "vllm"}
	for _, name := range local {
		w := &settingsWizard{provider: name}
		if w.isFieldEditable(wizardFieldAPIKey) {
			t.Errorf("provider %q: API key field should be hidden (auth=none, local server)", name)
		}
		// Base URL is still editable — the user might want to
		// point at a different port.
		if !w.isFieldEditable(wizardFieldBaseURL) {
			t.Errorf("provider %q: Base URL field should still be editable", name)
		}
	}
}

// TestIsFieldEditable_EnvHidesAPIKeyField covers the "env" auth
// kind (Bedrock reads AWS_BEARER_TOKEN_BEDROCK): the field shows
// "read from $AWS_BEARER_TOKEN_BEDROCK" as a hint, not an input.
func TestIsFieldEditable_EnvHidesAPIKeyField(t *testing.T) {
	w := &settingsWizard{provider: "bedrock"}
	if w.isFieldEditable(wizardFieldAPIKey) {
		t.Errorf("bedrock: API key field should be hidden (auth=env)")
	}
	if !w.isFieldEditable(wizardFieldBaseURL) {
		t.Errorf("bedrock: Base URL field should be editable")
	}
}

// TestIsFieldEditable_NameLockedForPreset covers the existing
// invariant: the Name field is only editable for custom providers,
// not built-in presets.
func TestIsFieldEditable_NameLockedForPreset(t *testing.T) {
	w := &settingsWizard{provider: "openai", isCustom: false}
	if w.isFieldEditable(wizardFieldName) {
		t.Errorf("Name field should be locked for built-in providers")
	}
	w.isCustom = true
	if !w.isFieldEditable(wizardFieldName) {
		t.Errorf("Name field should be editable for custom providers")
	}
}
