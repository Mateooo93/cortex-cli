package config

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func init() {
	// Use in-memory mock keyring for all tests.
	keyring.MockInit()
}

func TestResolveProviderKey_EnvVarWins(t *testing.T) {
	// Store a key in the keychain
	if err := StoreProviderKey("anthropic", "keychain-key"); err != nil {
		t.Fatalf("StoreProviderKey: %v", err)
	}
	defer DeleteProviderKey("anthropic")

	// Set env var — should take priority
	t.Setenv("ANTHROPIC_API_KEY", "env-key")

	key, source := ResolveProviderKey("anthropic", true)
	if key != "env-key" {
		t.Errorf("expected env-key, got %q", key)
	}
	if source != KeySourceEnv {
		t.Errorf("expected source %q, got %q", KeySourceEnv, source)
	}
}

func TestResolveProviderKey_KeychainFallback(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	if err := StoreProviderKey("anthropic", "keychain-key"); err != nil {
		t.Fatalf("StoreProviderKey: %v", err)
	}
	defer DeleteProviderKey("anthropic")

	key, source := ResolveProviderKey("anthropic", true)
	if key != "keychain-key" {
		t.Errorf("expected keychain-key, got %q", key)
	}
	if source != KeySourceKeychain {
		t.Errorf("expected source %q, got %q", KeySourceKeychain, source)
	}
}

func TestResolveProviderKey_NoneWhenEmpty(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	// Ensure no keychain entry
	DeleteProviderKey("anthropic")

	key, source := ResolveProviderKey("anthropic", true)
	if key != "" {
		t.Errorf("expected empty key, got %q", key)
	}
	if source != KeySourceNone {
		t.Errorf("expected source %q, got %q", KeySourceNone, source)
	}
}

func TestStoreAndResolveRoundTrip(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	if err := StoreProviderKey("anthropic", "roundtrip-key"); err != nil {
		t.Fatalf("StoreProviderKey: %v", err)
	}
	defer DeleteProviderKey("anthropic")

	key, source := ResolveProviderKey("anthropic", true)
	if key != "roundtrip-key" || source != KeySourceKeychain {
		t.Errorf("round-trip failed: key=%q source=%q", key, source)
	}
}

func TestDeleteProviderKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	StoreProviderKey("anthropic", "delete-me")
	if err := DeleteProviderKey("anthropic"); err != nil {
		t.Fatalf("DeleteProviderKey: %v", err)
	}

	key, source := ResolveProviderKey("anthropic", true)
	if key != "" || source != KeySourceNone {
		t.Errorf("expected empty after delete, got key=%q source=%q", key, source)
	}
}

func TestResolveProviderKey_OpenAI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-env-key")
	defer t.Setenv("OPENAI_API_KEY", "")

	key, source := ResolveProviderKey("openai", true)
	if key != "openai-env-key" {
		t.Errorf("expected openai-env-key, got %q", key)
	}
	if source != KeySourceEnv {
		t.Errorf("expected source %q, got %q", KeySourceEnv, source)
	}
}

func TestResolveProviderKey_OAuthSkippedWhenDisallowed(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "oauth-token-value")
	DeleteProviderKey("anthropic")

	key, source := ResolveProviderKey("anthropic", false)
	if key != "" {
		t.Errorf("expected empty key, got %q", key)
	}
	if source != KeySourceNone {
		t.Errorf("expected source %q, got %q", KeySourceNone, source)
	}
}

func TestResolveProviderKey_OAuthUsedWhenAllowed(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "oauth-token-value")
	DeleteProviderKey("anthropic")

	key, source := ResolveProviderKey("anthropic", true)
	if key != "oauth-token-value" {
		t.Errorf("expected oauth-token-value, got %q", key)
	}
	if source != KeySourceOAuthToken {
		t.Errorf("expected source %q, got %q", KeySourceOAuthToken, source)
	}
}

func TestResolveOAuthToken(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "my-oauth-token")

	key, source := ResolveOAuthToken()
	if key != "my-oauth-token" {
		t.Errorf("expected my-oauth-token, got %q", key)
	}
	if source != KeySourceOAuthToken {
		t.Errorf("expected source %q, got %q", KeySourceOAuthToken, source)
	}
}

func TestListStoredProviderKeys(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("MINIMAX_API_KEY", "")
	t.Setenv("MIMO_API_KEY", "")
	DeleteProviderKey("anthropic")
	DeleteProviderKey("openai")
	DeleteProviderKey("openrouter")
	DeleteProviderKey("minimax")
	DeleteProviderKey("mimo")

	StoreProviderKey("anthropic", "sk-ant-test-key")
	defer DeleteProviderKey("anthropic")

	keys := ListStoredProviderKeys()
	if len(keys) != 5 {
		t.Fatalf("expected 5 provider entries, got %d", len(keys))
	}

	anthropicFound := false
	for _, pk := range keys {
		if pk.Provider == "anthropic" {
			anthropicFound = true
			if pk.Prefix == "" {
				t.Errorf("expected non-empty prefix for anthropic")
			}
		}
		if pk.Provider == "openai" && pk.Prefix != "" {
			t.Errorf("expected empty prefix for openai (not stored)")
		}
	}
	if !anthropicFound {
		t.Errorf("anthropic not found in ListStoredProviderKeys")
	}
}
