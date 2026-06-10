package config

import (
	"strings"

	"github.com/zalando/go-keyring"
)

// oauthKeyringUsers maps subscription OAuth providers to their keychain entries.
var oauthKeyringUsers = map[string]string{
	"codex":      "codex-oauth-token",
	"claude-sub": "claude-code-oauth-token",
	"xai-sub":    "xai-sub-oauth-token",
}

// oauthEnvVars maps subscription OAuth providers to headless env fallbacks.
var oauthEnvVars = map[string]string{
	"codex":      "CODEX_CODEX_TOKEN",
	"claude-sub": "CLAUDE_CODE_OAUTH_TOKEN",
	"copilot":    "COPILOT_OAUTH_TOKEN",
	"xai-sub":    "XAI_OAUTH_TOKEN",
}

// OAuthProviderSignedIn reports whether a subscription OAuth provider has
// a stored token (keychain JSON bundle or env fallback).
func OAuthProviderSignedIn(provider string) bool {
	return OAuthProviderStatusPrefix(provider) != ""
}

// OAuthProviderStatusPrefix returns a short Settings-friendly status string:
// "(signed in)" for keychain tokens, "(env token)" for env fallbacks, or "".
func OAuthProviderStatusPrefix(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if user, ok := oauthKeyringUsers[provider]; ok {
		if raw, err := keyring.Get(keyringService, user); err == nil && raw != "" {
			return "(signed in)"
		}
	}
	if env, ok := oauthEnvVars[provider]; ok {
		if value, ok := ResolveEnvVar(env); ok && value != "" {
			return "(env token)"
		}
	}
	return ""
}
