package session

import "testing"

func TestResolveSubagentModelPrefersExplicitOverride(t *testing.T) {
	got := resolveSubagentModel("openai/gpt-4.1", "xai/grok-3")
	if got != "openai/gpt-4.1" {
		t.Fatalf("resolveSubagentModel = %q, want explicit override", got)
	}
}

func TestResolveSubagentModelUsesSessionActive(t *testing.T) {
	got := resolveSubagentModel("", "codex/gpt-5.4")
	if got != "codex/gpt-5.4" {
		t.Fatalf("resolveSubagentModel = %q, want session active model", got)
	}
}

func TestResolveSubagentModelTrimsOverride(t *testing.T) {
	got := resolveSubagentModel("  anthropic/claude-sonnet-4-6  ", "xai/grok-3")
	if got != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("resolveSubagentModel = %q, want trimmed override", got)
	}
}