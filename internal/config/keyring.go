package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/zalando/go-keyring"
)

const (
	keyringService = "cortex-cli"
)

// KeySource describes where the API key was found.
type KeySource string

const (
	KeySourceEnv        KeySource = "env"
	KeySourceOAuthToken KeySource = "oauth-token"
	KeySourceKeychain   KeySource = "keychain"
	KeySourceEnvFile    KeySource = "dotenv"
	KeySourceNone       KeySource = "none"
)

// Credential bundles an API key or OAuth token with its source.
// Use RequestOptions() to get the correct SDK auth options.
type Credential struct {
	Value  string
	Source KeySource
}

// RequestOptions returns the appropriate Anthropic SDK options for this credential.
func (c Credential) RequestOptions() []option.RequestOption {
	if c.Source == KeySourceOAuthToken {
		return []option.RequestOption{
			option.WithHeader("Authorization", "Bearer "+c.Value),
		}
	}
	return []option.RequestOption{option.WithAPIKey(c.Value)}
}

// ResolveEnvVar checks the environment and .env files for a variable.
// Returns the value and true if found, or empty string and false.
func ResolveEnvVar(name string) (string, bool) {
	if v := os.Getenv(name); v != "" {
		return v, true
	}
	if v := loadKeyFromEnvFile(loadExeEnvFilePath(), name); v != "" {
		return v, true
	}
	if v := loadKeyFromEnvFile(".env", name); v != "" {
		return v, true
	}
	return "", false
}

// ProviderKey holds a provider name and a display prefix of its stored key.
type ProviderKey struct {
	Provider string
	Prefix   string // first 10 chars of the stored key, for display; empty if not stored
}

// providerKeyringUser returns the keyring "user" field for a given provider.
// e.g. "anthropic" → "anthropic-api-key", "openai" → "openai-api-key"
func providerKeyringUser(provider string) string {
	return provider + "-api-key"
}

// providerEnvVar returns the environment variable name for a given provider.
func providerEnvVar(provider string) string {
	switch provider {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	case "minimax":
		return "MINIMAX_API_KEY"
	case "mimo":
		return "MIMO_API_KEY"
	default:
		return ""
	}
}

// resolveKey searches env var, OS keychain, and .env files for the given variable name
// and optional keyring user. Returns the value and source, or empty if not found.
func resolveKey(envVar, keyringUser string) (string, KeySource) {
	// 1. Environment variable
	if envVar != "" {
		if key := os.Getenv(envVar); key != "" {
			return key, KeySourceEnv
		}
	}

	// 2. OS Keychain
	if keyringUser != "" {
		if key, err := keyring.Get(keyringService, keyringUser); err == nil && key != "" {
			return key, KeySourceKeychain
		}
	}

	// 3. .env next to executable
	if envVar != "" {
		if key := loadKeyFromEnvFile(loadExeEnvFilePath(), envVar); key != "" {
			return key, KeySourceEnvFile
		}

		// 4. .env in CWD
		if key := loadKeyFromEnvFile(".env", envVar); key != "" {
			return key, KeySourceEnvFile
		}
	}

	return "", KeySourceNone
}

// ResolveOAuthToken resolves the CLAUDE_CODE_OAUTH_TOKEN through the standard
// source chain (env var → keychain → .env) and returns its value and source.
func ResolveOAuthToken() (string, KeySource) {
	key, _ := resolveKey("CLAUDE_CODE_OAUTH_TOKEN", "claude-code-oauth-token")
	if key != "" {
		return key, KeySourceOAuthToken
	}
	return "", KeySourceNone
}

// ResolveProviderKey checks all sources in priority order and returns the key and its source.
// For anthropic, ANTHROPIC_API_KEY is checked across all sources first, then
// CLAUDE_CODE_OAUTH_TOKEN is checked across all sources as a fallback (only when allowOAuth is true).
func ResolveProviderKey(provider string, allowOAuth bool) (key string, source KeySource) {
	envVar := providerEnvVar(provider)
	key, source = resolveKey(envVar, providerKeyringUser(provider))
	if key != "" {
		return key, source
	}

	// Fall back to Claude Code OAuth token (anthropic only)
	if allowOAuth && provider == "anthropic" {
		key, source = resolveKey("CLAUDE_CODE_OAUTH_TOKEN", "claude-code-oauth-token")
		if key != "" {
			return key, KeySourceOAuthToken
		}
	}

	return "", KeySourceNone
}

// ResolveProviderCredential returns a Credential for the given provider.
func ResolveProviderCredential(provider string, allowOAuth bool) Credential {
	key, source := ResolveProviderKey(provider, allowOAuth)
	return Credential{Value: key, Source: source}
}

// StoreProviderKey writes the API key for the given provider to the OS keychain.
func StoreProviderKey(provider, key string) error {
	return keyring.Set(keyringService, providerKeyringUser(provider), key)
}

// DeleteProviderKey removes the API key for the given provider from the OS keychain.
func DeleteProviderKey(provider string) error {
	return keyring.Delete(keyringService, providerKeyringUser(provider))
}

// ListStoredProviderKeys returns the stored key info for all known providers.
// The Prefix field holds the first 10 chars of the stored key (empty if not stored).
func ListStoredProviderKeys() []ProviderKey {
	providers := []string{"anthropic", "openai", "openrouter", "minimax", "mimo"}
	result := make([]ProviderKey, 0, len(providers))
	for _, p := range providers {
		pk := ProviderKey{Provider: p}
		if k, err := keyring.Get(keyringService, providerKeyringUser(p)); err == nil && k != "" {
			if len(k) > 10 {
				pk.Prefix = k[:10]
			} else {
				pk.Prefix = k
			}
		}
		result = append(result, pk)
	}
	// Codex uses its own keychain user (codex-oauth-token) and stores
	// a JSON Token rather than a plain API key. Surface it as
	// "(oauth)" so the Settings tab KeyManager can list it alongside
	// the rest, and Del-signOut works through the codex package.
	if k, err := keyring.Get(keyringService, "codex-oauth-token"); err == nil && k != "" {
		// Cheap heuristic: a stored token starts with {"access_token":".
		if strings.HasPrefix(k, `{"access_token":`) {
			result = append(result, ProviderKey{Provider: "codex", Prefix: "(oauth)"})
		}
	}
	return result
}

// loadExeEnvFilePath returns the path to the .env file next to the executable.
func loadExeEnvFilePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "..", "..", ".env")
}

// loadKeyFromEnvFile reads a .env file and extracts the value of the given variable name.
func loadKeyFromEnvFile(path, varName string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	prefix := varName + "="
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.SplitN(line, "=", 2)[1])
		}
	}
	return ""
}
