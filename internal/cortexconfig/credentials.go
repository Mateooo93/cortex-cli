package cortexconfig

import (
	"os"
	"strings"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig/zenkey"
)

const (
	// BuiltinZenProvider is the bundled free-tier OpenCode Zen provider.
	BuiltinZenProvider = "opencode-zen"
	// BuiltinZenModel is the default free model on OpenCode Zen.
	BuiltinZenModel = "mimo-v2.5-free"
)

// ResolveProviderAPIKey returns the effective API key for provider, checking
// model config, provider config, environment, then the bundled Zen key.
func ResolveProviderAPIKey(cfg *Config, provider string, mc *ModelConfig) string {
	provider = NormalizeProviderName(provider)
	if mc != nil && strings.TrimSpace(mc.APIKey) != "" {
		return strings.TrimSpace(mc.APIKey)
	}
	if cfg != nil {
		if pc, ok := cfg.ProviderConfig(provider); ok && strings.TrimSpace(pc.APIKey) != "" {
			return strings.TrimSpace(pc.APIKey)
		}
	}
	if envVar := ProviderEnvVar(provider); envVar != "" {
		if v := strings.TrimSpace(os.Getenv(envVar)); v != "" {
			return v
		}
	}
	if provider == BuiltinZenProvider {
		return strings.TrimSpace(zenkey.Key())
	}
	return ""
}

// HasUsableCredentials reports whether the active default model can run
// without prompting the user for an API key.
func (c *Config) HasUsableCredentials() bool {
	if c == nil {
		return false
	}
	_, mc, err := c.GetModel(c.DefaultModel)
	if err != nil || mc == nil {
		return false
	}
	providerName := NormalizeProviderName(mc.Provider)
	if !ProviderNeedsAPIKey(providerName) {
		return true
	}
	return ResolveProviderAPIKey(c, providerName, mc) != ""
}

// ApplyBuiltinZenFallback switches fresh installs to the bundled OpenCode Zen
// free tier when no other provider credentials are configured.
func (c *Config) ApplyBuiltinZenFallback() {
	if c == nil || !zenkey.Available() {
		return
	}
	if c.HasUsableCredentials() {
		return
	}
	c.EnsureProviderPresets()
	c.DefaultModel = BuiltinZenProvider
	if spec := c.EnsureProviderModel(BuiltinZenProvider, BuiltinZenModel); spec != "" {
		c.DefaultModel = spec
	}
}
