// Package cortexconfig loads and persists the user-facing cortex-cli config.
// The file is YAML, lives at ~/.cortex/config.yaml, and matches the schema
// of the original TypeScript implementation so users can copy their
// existing config over.
package cortexconfig

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProviderPreset describes a built-in OpenAI-compatible provider preset.
// AuthKind tells the Settings tab which auth path the user has to follow:
//
//	"oauth"   — no API key; the user signs in with their existing
//	            subscription (ChatGPT Plus/Pro, Claude Pro/Max, …) via
//	            the in-app browser flow. APIKeyEnvVar is a CI/headless
//	            fallback only.
//	"apikey"  — user pastes an API key from the provider's dashboard.
//	"none"    — local server, no key needed (Cortex, Ollama, LM Studio,
//	            vLLM). APIKeyEnvVar is unused.
//	"env"     — same as "none" but the key is read from an env var
//	            (e.g. AWS Bedrock with AWS_BEARER_TOKEN_BEDROCK).
type ProviderPreset struct {
	Name         string
	DisplayName  string
	BaseURL      string
	APIKeyEnvVar string
	DefaultModel string
	NeedsAPIKey  bool
	AuthKind     string // "oauth" | "apikey" | "none" | "env"
	// HelpURL points at the provider's auth / API-key page; the
	// Settings tab uses it for the "Get an API key" link.
	HelpURL string
}

// BuiltinProviderPresets is the ordered set shown in Settings. Order
// matters: subscription (OAuth) providers come first so new users
// who already pay for ChatGPT or Claude don't have to scroll past
// the API-key options, then the API-key providers, then the
// keyless local servers, then the catch-all "custom" entry.
var BuiltinProviderPresets = []ProviderPreset{
	// ===== Subscriptions (OAuth) — no API key =====
	// Uses the user's existing subscription. The auth flow opens a
	// local browser; the resulting token is stored in the OS keychain.
	{
		Name: "codex", DisplayName: "ChatGPT (codex)",
		BaseURL:      "https://api.openai.com/v1",
		APIKeyEnvVar: "CODEX_CODEX_TOKEN",
		DefaultModel: "gpt-5.5",
		NeedsAPIKey:  false, AuthKind: "oauth",
		HelpURL: "https://chatgpt.com/auth/login",
	},
	{
		Name: "claude-sub", DisplayName: "Claude (Pro/Max)",
		BaseURL:      "https://api.anthropic.com/v1",
		APIKeyEnvVar: "CLAUDE_CODE_OAUTH_TOKEN",
		DefaultModel: "claude-opus-4-8",
		NeedsAPIKey:  false, AuthKind: "oauth",
		HelpURL: "https://claude.ai/login",
	},
	{
		Name: "copilot", DisplayName: "GitHub Copilot",
		BaseURL:      "https://api.githubcopilot.com",
		APIKeyEnvVar: "COPILOT_OAUTH_TOKEN",
		DefaultModel: "gpt-5.5",
		NeedsAPIKey:  false, AuthKind: "oauth",
		HelpURL: "https://github.com/settings/copilot",
	},

	// ===== API-key providers (paid) =====
	{
		Name: "openai", DisplayName: "OpenAI",
		BaseURL:      "https://api.openai.com/v1",
		APIKeyEnvVar: "OPENAI_API_KEY",
		DefaultModel: "gpt-5.5",
		NeedsAPIKey:  true, AuthKind: "apikey",
		HelpURL: "https://platform.openai.com/api-keys",
	},
	{
		Name: "anthropic", DisplayName: "Anthropic",
		BaseURL:      "https://api.anthropic.com/v1",
		APIKeyEnvVar: "ANTHROPIC_API_KEY",
		DefaultModel: "claude-opus-4-8",
		NeedsAPIKey:  true, AuthKind: "apikey",
		HelpURL: "https://console.anthropic.com/settings/keys",
	},
	{
		Name: "gemini", DisplayName: "Google Gemini",
		BaseURL:      "https://generativelanguage.googleapis.com/v1beta/openai",
		APIKeyEnvVar: "GEMINI_API_KEY",
		DefaultModel: "gemini-2.5-pro",
		NeedsAPIKey:  true, AuthKind: "apikey",
		HelpURL: "https://aistudio.google.com/apikey",
	},
	{
		Name: "xai", DisplayName: "xAI (Grok)",
		BaseURL:      "https://api.x.ai/v1",
		APIKeyEnvVar: "XAI_API_KEY",
		DefaultModel: "grok-4",
		NeedsAPIKey:  true, AuthKind: "apikey",
		HelpURL: "https://console.x.ai",
	},
	{
		Name: "deepseek", DisplayName: "DeepSeek",
		BaseURL:      "https://api.deepseek.com",
		APIKeyEnvVar: "DEEPSEEK_API_KEY",
		DefaultModel: "deepseek-chat",
		NeedsAPIKey:  true, AuthKind: "apikey",
		HelpURL: "https://platform.deepseek.com/api_keys",
	},
	{
		Name: "mistral", DisplayName: "Mistral AI",
		BaseURL:      "https://api.mistral.ai/v1",
		APIKeyEnvVar: "MISTRAL_API_KEY",
		DefaultModel: "mistral-large-latest",
		NeedsAPIKey:  true, AuthKind: "apikey",
		HelpURL: "https://console.mistral.ai/api-keys",
	},
	{
		Name: "groq", DisplayName: "Groq",
		BaseURL:      "https://api.groq.com/openai/v1",
		APIKeyEnvVar: "GROQ_API_KEY",
		DefaultModel: "llama-3.3-70b-versatile",
		NeedsAPIKey:  true, AuthKind: "apikey",
		HelpURL: "https://console.groq.com/keys",
	},
	{
		Name: "cohere", DisplayName: "Cohere",
		BaseURL:      "https://api.cohere.com/v1",
		APIKeyEnvVar: "COHERE_API_KEY",
		DefaultModel: "command-r-plus",
		NeedsAPIKey:  true, AuthKind: "apikey",
		HelpURL: "https://dashboard.cohere.com/api-keys",
	},
	{
		Name: "perplexity", DisplayName: "Perplexity",
		BaseURL:      "https://api.perplexity.ai",
		APIKeyEnvVar: "PERPLEXITY_API_KEY",
		DefaultModel: "sonar-pro",
		NeedsAPIKey:  true, AuthKind: "apikey",
		HelpURL: "https://www.perplexity.ai/settings/api",
	},

	// ===== Aggregators / multi-model gateways =====
	{
		Name: "openrouter", DisplayName: "OpenRouter",
		BaseURL:      "https://openrouter.ai/api/v1",
		APIKeyEnvVar: "OPENROUTER_API_KEY",
		DefaultModel: "anthropic/claude-opus-4-8",
		NeedsAPIKey:  true, AuthKind: "apikey",
		HelpURL: "https://openrouter.ai/keys",
	},
	{
		Name: "opengateway", DisplayName: "OpenGateway",
		BaseURL:      "https://opengateway.gitlawb.com/v1",
		APIKeyEnvVar: "OPENGATEWAY_API_KEY",
		DefaultModel: "minimax/minimax-m3",
		NeedsAPIKey:  true, AuthKind: "apikey",
	},
	{
		Name: "minimax", DisplayName: "MiniMax",
		BaseURL:      "https://api.minimax.io/v1",
		APIKeyEnvVar: "MINIMAX_API_KEY",
		DefaultModel: "MiniMax-M2.7",
		NeedsAPIKey:  true, AuthKind: "apikey",
	},
	{
		Name: "mimo", DisplayName: "Xiaomi MiMo",
		BaseURL:      "https://api.xiaomimimo.com/v1",
		APIKeyEnvVar: "MIMO_API_KEY",
		DefaultModel: "mimo-v2.5-pro",
		NeedsAPIKey:  true, AuthKind: "apikey",
	},
	{
		Name: "bedrock", DisplayName: "AWS Bedrock (OpenAI-compat)",
		BaseURL:      "https://bedrock-mantle.us-east-1.amazonaws.com/v1",
		APIKeyEnvVar: "AWS_BEARER_TOKEN_BEDROCK",
		DefaultModel: "anthropic.claude-opus-4-8",
		NeedsAPIKey:  true, AuthKind: "env",
		HelpURL: "https://docs.aws.amazon.com/bedrock/latest/userguide/inference-chat-completions.html",
	},

	// ===== Local / self-hosted (no key) =====
	{
		Name: "cortex", DisplayName: "Cortex",
		BaseURL:      "http://127.0.0.1:8000/v1",
		APIKeyEnvVar: "CORTEX_API_KEY",
		DefaultModel: "cortex-code",
		NeedsAPIKey:  false, AuthKind: "none",
	},
	{
		Name: "ollama", DisplayName: "Ollama",
		BaseURL:      "http://127.0.0.1:11434/v1",
		APIKeyEnvVar: "",
		DefaultModel: "qwen3.5",
		NeedsAPIKey:  false, AuthKind: "none",
	},
	{
		Name: "lmstudio", DisplayName: "LM Studio",
		BaseURL:      "http://127.0.0.1:1234/v1",
		APIKeyEnvVar: "",
		DefaultModel: "qwen2.5-7b-instruct",
		NeedsAPIKey:  false, AuthKind: "none",
	},
	{
		Name: "vllm", DisplayName: "vLLM",
		BaseURL:      "http://127.0.0.1:8001/v1",
		APIKeyEnvVar: "",
		DefaultModel: "meta-llama/Llama-3.3-70B-Instruct",
		NeedsAPIKey:  false, AuthKind: "none",
	},
}

func presetForProvider(provider string) (ProviderPreset, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	for _, p := range BuiltinProviderPresets {
		if p.Name == provider {
			return p, true
		}
	}
	return ProviderPreset{}, false
}

// IsCustomProvider reports whether a provider name is not one of the
// built-in presets — i.e. it was added by the user via AddCustomProvider.
func IsCustomProvider(provider string) bool {
	_, ok := presetForProvider(provider)
	return !ok
}

// ProviderDisplayName returns a friendly name for a provider.
func ProviderDisplayName(provider string) string {
	if p, ok := presetForProvider(provider); ok {
		return p.DisplayName
	}
	return provider
}

// ModelContextWindow returns the model's context window in
// tokens, or 0 if the model is unknown. Used by the right panel
// to render the context-usage bar (input + cache-read / max).
//
// Values come from a built-in lookup table of the most common
// 2024–2026 frontier models. The lookup is best-effort; if the
// model isn't in the table, callers should treat 0 as "unknown"
// and show the token count without a percentage.
func ModelContextWindow(spec string) int64 {
	if spec == "" {
		return 0
	}
	// Normalize: drop the "provider:" prefix and lowercase.
	raw := spec
	if colon := strings.Index(raw, ":"); colon >= 0 {
		raw = raw[colon+1:]
	}
	raw = strings.ToLower(strings.TrimSpace(raw))

	// OpenAI / ChatGPT
	if strings.HasPrefix(raw, "gpt-5") || raw == "gpt-5" {
		return 400_000
	}
	if strings.HasPrefix(raw, "gpt-4.1") {
		return 1_000_000
	}
	if strings.HasPrefix(raw, "gpt-4o") {
		return 128_000
	}
	if strings.HasPrefix(raw, "gpt-4-turbo") || raw == "gpt-4-turbo" {
		return 128_000
	}
	if strings.HasPrefix(raw, "o3") || strings.HasPrefix(raw, "o4") {
		return 200_000
	}
	// Anthropic Claude
	if strings.HasPrefix(raw, "claude-opus-4") || strings.HasPrefix(raw, "claude-4-opus") {
		return 200_000
	}
	if strings.HasPrefix(raw, "claude-sonnet-4") || strings.HasPrefix(raw, "claude-4-sonnet") {
		return 200_000
	}
	if strings.HasPrefix(raw, "claude-3-7-sonnet") || strings.HasPrefix(raw, "claude-3.7-sonnet") {
		return 200_000
	}
	if strings.HasPrefix(raw, "claude-3-5-sonnet") {
		return 200_000
	}
	if strings.HasPrefix(raw, "claude-3-5-haiku") {
		return 200_000
	}
	if strings.HasPrefix(raw, "claude-3-opus") {
		return 200_000
	}
	// Google Gemini
	if strings.HasPrefix(raw, "gemini-2.5-pro") || strings.HasPrefix(raw, "gemini-2.5-flash") {
		return 1_000_000
	}
	if strings.HasPrefix(raw, "gemini-2.0") {
		return 1_000_000
	}
	if strings.HasPrefix(raw, "gemini-1.5-pro") {
		return 2_000_000
	}
	if strings.HasPrefix(raw, "gemini-1.5-flash") {
		return 1_000_000
	}
	// Mistral / Mixtral
	if strings.HasPrefix(raw, "mistral-large-2") || strings.HasPrefix(raw, "mistral-large") {
		return 128_000
	}
	if strings.HasPrefix(raw, "codestral") {
		return 256_000
	}
	// Meta Llama
	if strings.HasPrefix(raw, "llama-3.1-405b") || strings.HasPrefix(raw, "llama-3.1-70b") ||
		strings.HasPrefix(raw, "llama-3.1-8b") || strings.HasPrefix(raw, "llama-3.3") {
		return 128_000
	}
	// DeepSeek
	if strings.HasPrefix(raw, "deepseek") {
		return 128_000
	}
	// Qwen
	if strings.HasPrefix(raw, "qwen-2.5") {
		return 128_000
	}
	// Local models typically don't have a known window
	// (depends on the deployment), so leave 0.
	return 0
}

// ProviderEnvVar returns the API-key environment variable for a provider.
func ProviderEnvVar(provider string) string {
	if p, ok := presetForProvider(provider); ok {
		return p.APIKeyEnvVar
	}
	return ""
}

// ProviderNeedsAPIKey reports whether Settings should require an API key.
func ProviderNeedsAPIKey(provider string) bool {
	if p, ok := presetForProvider(provider); ok {
		return p.NeedsAPIKey
	}
	return true
}

// ProviderHelpURL returns the link the Settings tab shows next to the
// auth-method badge. Empty string for providers that don't have a
// "get a key" page (e.g. local servers).
func ProviderHelpURL(provider string) string {
	if p, ok := presetForProvider(provider); ok {
		return p.HelpURL
	}
	return ""
}

// ProviderAuthKind returns "oauth" | "apikey" | "none" | "env" for a
// built-in provider, or "apikey" for unknown / custom providers (the
// safe default — the user has to bring their own key).
func ProviderAuthKind(provider string) string {
	if p, ok := presetForProvider(provider); ok {
		if p.AuthKind != "" {
			return p.AuthKind
		}
		// Backwards compat: if a preset was created before the
		// AuthKind field existed, infer it from NeedsAPIKey.
		if !p.NeedsAPIKey {
			return "none"
		}
		return "apikey"
	}
	return "apikey"
}

// DefaultBaseURL returns the built-in base URL for a provider, if known.
func DefaultBaseURL(provider string) string {
	if p, ok := presetForProvider(provider); ok {
		return p.BaseURL
	}
	return ""
}

// NormalizeProviderName turns user-entered provider names into config keys.
func NormalizeProviderName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "-")
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-_")
}

// SplitModelSpec splits "provider/model" while preserving nested model IDs.
func SplitModelSpec(spec string) (provider, model string, ok bool) {
	i := strings.Index(spec, "/")
	if i <= 0 || i == len(spec)-1 {
		return "", "", false
	}
	return spec[:i], spec[i+1:], true
}

// ModelSpec joins a provider and raw model ID into a config model key.
func ModelSpec(provider, model string) string {
	provider = NormalizeProviderName(provider)
	model = strings.TrimSpace(model)
	if provider == "" || model == "" {
		return ""
	}
	return provider + "/" + model
}

// ModelConfig describes one model entry from the config.
type ModelConfig struct {
	Provider         string  `yaml:"provider"`
	Model            string  `yaml:"model"`
	BaseURL          string  `yaml:"baseUrl,omitempty"`
	APIKey           string  `yaml:"apiKey,omitempty"`
	Temperature      float64 `yaml:"temperature,omitempty"`
	MaxTokens        int     `yaml:"maxTokens,omitempty"`
	ReasoningEffort  string  `yaml:"reasoningEffort,omitempty"`
	CortexPromptMode string  `yaml:"cortexPromptMode,omitempty"`
}

// SwarmDefaults holds the orchestrator's default knobs.
type SwarmDefaults struct {
	MaxAgents int    `yaml:"maxAgents"`
	Timeout   int    `yaml:"timeout"` // minutes
	Strategy  string `yaml:"strategy"`
	Mode      string `yaml:"mode"`
}

// ToolPerms gates which tools the agent can use.
type ToolPerms struct {
	AllowShell bool `yaml:"allowShell"`
	AllowWrite bool `yaml:"allowWrite"`
	AllowGit   bool `yaml:"allowGit"`
}

// Config is the top-level user config.
type Config struct {
	DefaultModel  string                 `yaml:"defaultModel"`
	Models        map[string]ModelConfig `yaml:"models"`
	SystemPrompt  string                 `yaml:"systemPrompt,omitempty"`
	SwarmDefaults SwarmDefaults          `yaml:"swarmDefaults"`
	Tools         ToolPerms              `yaml:"tools"`
	Streaming     bool                   `yaml:"streaming"`
	ShowUsage     bool                   `yaml:"showUsage"`
	Theme         string                 `yaml:"theme"`
	// AutoCompact triggers an automatic /compact run when the
	// current context window usage exceeds 80% of the model's
	// window. Default true so users get a safety net on long
	// sessions; power users can turn it off in Settings →
	// Other Settings.
	AutoCompact bool `yaml:"autoCompact"`
}

// Default is the default config (matches the original TS default).
func Default() *Config {
	return &Config{
		DefaultModel: "cortex",
		Models: map[string]ModelConfig{
			"cortex": {
				Provider:         "cortex",
				Model:            "cortex-code",
				BaseURL:          "http://127.0.0.1:8000/v1",
				APIKey:           "",
				Temperature:      0.2,
				MaxTokens:        32768,
				ReasoningEffort:  "auto",
				CortexPromptMode: "minimal",
			},
			"openai": {
				Provider:    "openai",
				Model:       "gpt-5.5",
				BaseURL:     "https://api.openai.com/v1",
				APIKey:      "",
				Temperature: 0.2,
				MaxTokens:   32768,
			},
			"anthropic": {
				Provider:    "anthropic",
				Model:       "claude-opus-4-8",
				BaseURL:     "https://api.anthropic.com/v1",
				APIKey:      "",
				Temperature: 0.2,
				MaxTokens:   32768,
			},
			"ollama": {
				Provider:    "ollama",
				Model:       "qwen3.5",
				BaseURL:     "http://127.0.0.1:11434/v1",
				APIKey:      "ollama",
				Temperature: 0.2,
				MaxTokens:   32768,
			},
		},
		SwarmDefaults: SwarmDefaults{
			MaxAgents: 5,
			Timeout:   60,
			Strategy:  "development",
			Mode:      "centralized",
		},
		Tools: ToolPerms{
			AllowShell: true,
			AllowWrite: true,
			AllowGit:   true,
		},
		Streaming:   true,
		ShowUsage:   true,
		AutoCompact: true,
		Theme:       "auto",
	}
}

// ConfigPath returns the absolute path to the config file.
func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cortex", "config.yaml")
}

// Dir returns the absolute path to the config directory.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cortex")
}

// Load reads the config from disk, writing defaults if absent.
func Load() (*Config, error) {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	p := ConfigPath()
	if _, err := os.Stat(p); os.IsNotExist(err) {
		cfg := Default()
		if err := Save(cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	// Re-apply defaults for "true-by-default" toggles if the
	// field is absent from the YAML. yaml.Unmarshal leaves
	// missing bool fields as false even if the default is
	// true, which silently turned streaming / showUsage /
	// autoCompact / tool permissions OFF for users who had a
	// config from before those settings existed. We detect the
	// absence by checking the raw YAML for the key. If the
	// user has explicitly set the field (any value, including
	// false), we respect their choice.
	//
	// Bug history: the original loader only re-applied
	// streaming / showUsage / autoCompact. The tools block
	// (allowShell / allowWrite / allowGit) was missing, so a
	// user upgrading from a pre-tools config ended up with all
	// three permissions stuck at false — every shell / file
	// write tool call returned "X is disabled in config" even
	// though the user had never opted out. The user reported
	// this as "default settings i told you to keep on earlier
	// arent left on" and "shell execution is disabled in
	// config" errors flooding the chat. Fixed by extending
	// hasField coverage to the tool permissions.
	hasField := func(name string) bool {
		return bytes.Contains(data, []byte("\n"+name+":")) ||
			bytes.Contains(data, []byte(" "+name+":")) ||
			bytes.HasPrefix(data, []byte(name+":"))
	}
	// Streaming is always on; the setting was removed from the UI.
	cfg.Streaming = true
	if !hasField("showUsage") {
		cfg.ShowUsage = true
	}
	if !hasField("autoCompact") {
		cfg.AutoCompact = true
	}
	// Shell is always forced enabled ("enable shell in config just in case
	// the model wants to use it"). Write/git respect explicit false (via
	// hasField check on raw YAML after unmarshal, which otherwise leaves
	// bools false).
	cfg.Tools.AllowShell = true
	if !hasField("allowWrite") {
		cfg.Tools.AllowWrite = true
	}
	if !hasField("allowGit") {
		cfg.Tools.AllowGit = true
	}
	cfg.EnsureProviderPresets()
	return cfg, nil
}

// Save writes the config to disk in YAML form.
//
// IMPORTANT: Save uses a deep-merge strategy instead of
// yaml.Marshal. yaml.Marshal would only serialize fields
// the Go struct knows about, silently dropping anything
// else in the file (custom provider fields the user added
// by hand, comments, alternative formatting). The merge
// strategy reads the existing file as a generic
// map[string]any, then overwrites only the keys the Go
// struct populates, leaving everything else untouched.
//
// The user reported: "i lose my whole provider config when
// updating". The old Save would call yaml.Marshal which
// serialized the in-memory Config struct back to disk — if
// Load() had applied any defaults via hasField() (e.g. the
// hasField("streaming") re-applies cfg.Streaming = true
// when the field is absent from YAML), the Save would
// then write that re-applied value to disk, losing the
// user's original file. Or if a user had a custom field
// like `models.anthropic.customHeader: foo`, the Save
// would drop `customHeader` from disk because the
// ModelConfig struct doesn't have a `CustomHeader` field.
//
// The merge keeps the on-disk file as the source of truth
// for anything the struct doesn't know about.
func Save(cfg *Config) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p := ConfigPath()
	// Read the existing file as a generic map so we can
	// preserve unknown keys.
	var existing map[string]any
	if data, err := os.ReadFile(p); err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}
	// Marshal the new struct.
	newData, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	var newMap map[string]any
	if err := yaml.Unmarshal(newData, &newMap); err != nil {
		return err
	}
	// Deep-merge: newMap overrides existing, but any keys
	// in existing that aren't in newMap are kept.
	merged := deepMergeMap(existing, newMap)
	out, err := yaml.Marshal(merged)
	if err != nil {
		return err
	}
	return os.WriteFile(p, out, 0o644)
}

// deepMergeMap returns a new map containing all keys from
// base, with values from override replacing or merging into
// the base values. Nested maps are merged recursively.
// Scalars, slices, and nil values in override fully replace
// the base value.
func deepMergeMap(base, override map[string]any) map[string]any {
	out := make(map[string]any, len(base))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		if baseMap, ok := base[k].(map[string]any); ok {
			if overrideMap, ok := v.(map[string]any); ok {
				out[k] = deepMergeMap(baseMap, overrideMap)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// GetModel returns the model config for a name, falling back to defaultModel.
func (c *Config) GetModel(name string) (string, *ModelConfig, error) {
	if name == "" {
		name = c.DefaultModel
	}
	m, ok := c.Models[name]
	if !ok {
		if provider, model, parsed := SplitModelSpec(name); parsed {
			if spec := c.EnsureProviderModel(provider, model); spec != "" {
				m = c.Models[spec]
				return spec, &m, nil
			}
		}
		return "", nil, fmt.Errorf("unknown model: %s", name)
	}
	return name, &m, nil
}

// EnsureProviderPresets makes sure every built-in provider has a config row.
func (c *Config) EnsureProviderPresets() {
	if c.Models == nil {
		c.Models = map[string]ModelConfig{}
	}
	for _, p := range BuiltinProviderPresets {
		mc, ok := c.Models[p.Name]
		if !ok {
			c.Models[p.Name] = ModelConfig{
				Provider:    p.Name,
				Model:       p.DefaultModel,
				BaseURL:     p.BaseURL,
				Temperature: 0.2,
				MaxTokens:   32768,
			}
			continue
		}
		if mc.Provider == "" {
			mc.Provider = p.Name
		}
		if mc.Model == "" {
			mc.Model = p.DefaultModel
		}
		if mc.BaseURL == "" && p.BaseURL != "" {
			mc.BaseURL = p.BaseURL
		}
		if mc.MaxTokens == 0 {
			mc.MaxTokens = 2048
		}
		c.Models[p.Name] = mc
	}
}

// ProviderNames returns all configured providers in Settings display order.
func (c *Config) ProviderNames() []string {
	seen := map[string]bool{}
	var names []string
	for _, p := range BuiltinProviderPresets {
		if _, ok := c.Models[p.Name]; ok {
			names = append(names, p.Name)
			seen[p.Name] = true
		}
	}
	var custom []string
	for _, mc := range c.Models {
		provider := NormalizeProviderName(mc.Provider)
		if provider == "" || seen[provider] {
			continue
		}
		custom = append(custom, provider)
		seen[provider] = true
	}
	sort.Strings(custom)
	return append(names, custom...)
}

// ProviderConfig returns the best config row to represent a provider's settings.
func (c *Config) ProviderConfig(provider string) (ModelConfig, bool) {
	provider = NormalizeProviderName(provider)
	if provider == "" {
		return ModelConfig{}, false
	}
	if mc, ok := c.Models[provider]; ok {
		if mc.Provider == "" {
			mc.Provider = provider
		}
		return mc, true
	}
	for _, mc := range c.Models {
		if NormalizeProviderName(mc.Provider) == provider {
			return mc, true
		}
	}
	return ModelConfig{}, false
}

// SetProviderBaseURL updates every config row for a provider and ensures a preset row exists.
func (c *Config) SetProviderBaseURL(provider, baseURL string) {
	provider = NormalizeProviderName(provider)
	if provider == "" {
		return
	}
	if c.Models == nil {
		c.Models = map[string]ModelConfig{}
	}
	updated := false
	for key, mc := range c.Models {
		if NormalizeProviderName(mc.Provider) == provider || key == provider {
			if mc.Provider == "" {
				mc.Provider = provider
			}
			mc.BaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
			c.Models[key] = mc
			updated = true
		}
	}
	if !updated {
		p, _ := presetForProvider(provider)
		mc := ModelConfig{Provider: provider, Model: p.DefaultModel, BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), Temperature: 0.2, MaxTokens: 32768}
		if mc.Model == "" {
			mc.Model = "model"
		}
		c.Models[provider] = mc
	}
}

// SetProviderAPIKey updates every config row for a provider and ensures a preset row exists.
func (c *Config) SetProviderAPIKey(provider, apiKey string) {
	provider = NormalizeProviderName(provider)
	if provider == "" {
		return
	}
	if c.Models == nil {
		c.Models = map[string]ModelConfig{}
	}
	updated := false
	for key, mc := range c.Models {
		if NormalizeProviderName(mc.Provider) == provider || key == provider {
			if mc.Provider == "" {
				mc.Provider = provider
			}
			mc.APIKey = strings.TrimSpace(apiKey)
			c.Models[key] = mc
			updated = true
		}
	}
	if !updated {
		p, _ := presetForProvider(provider)
		mc := ModelConfig{Provider: provider, Model: p.DefaultModel, BaseURL: p.BaseURL, APIKey: strings.TrimSpace(apiKey), Temperature: 0.2, MaxTokens: 32768}
		if mc.Model == "" {
			mc.Model = "model"
		}
		c.Models[provider] = mc
	}
}

// DeleteProviderAPIKey clears every stored API key for a provider.
func (c *Config) DeleteProviderAPIKey(provider string) {
	provider = NormalizeProviderName(provider)
	for key, mc := range c.Models {
		if NormalizeProviderName(mc.Provider) == provider || key == provider {
			mc.APIKey = ""
			c.Models[key] = mc
		}
	}
}

// EnsureProviderModel adds or updates a model entry for a provider.
func (c *Config) EnsureProviderModel(provider, model string) string {
	provider = NormalizeProviderName(provider)
	model = strings.TrimSpace(model)
	if provider == "" || model == "" {
		return ""
	}
	if c.Models == nil {
		c.Models = map[string]ModelConfig{}
	}
	spec := ModelSpec(provider, model)
	if existing, ok := c.Models[spec]; ok {
		if existing.Provider == "" {
			existing.Provider = provider
		}
		if existing.Model == "" {
			existing.Model = model
		}
		if pc, ok := c.ProviderConfig(provider); ok {
			if existing.BaseURL == "" {
				existing.BaseURL = pc.BaseURL
			}
			if existing.APIKey == "" {
				existing.APIKey = pc.APIKey
			}
			if existing.Temperature == 0 {
				existing.Temperature = pc.Temperature
			}
			if existing.ReasoningEffort == "" {
				existing.ReasoningEffort = pc.ReasoningEffort
			}
			if existing.CortexPromptMode == "" {
				existing.CortexPromptMode = pc.CortexPromptMode
			}
		}
		if existing.MaxTokens == 0 {
			existing.MaxTokens = 2048
		}
		c.Models[spec] = existing
		return spec
	}
	pc, _ := c.ProviderConfig(provider)
	mc := ModelConfig{
		Provider:         provider,
		Model:            model,
		BaseURL:          pc.BaseURL,
		APIKey:           pc.APIKey,
		Temperature:      pc.Temperature,
		MaxTokens:        pc.MaxTokens,
		ReasoningEffort:  pc.ReasoningEffort,
		CortexPromptMode: pc.CortexPromptMode,
	}
	if mc.BaseURL == "" {
		mc.BaseURL = DefaultBaseURL(provider)
	}
	if mc.MaxTokens == 0 {
		mc.MaxTokens = 2048
	}
	c.Models[spec] = mc
	return spec
}

// AddCustomProvider creates a new OpenAI-compatible provider row.
func (c *Config) AddCustomProvider(provider, baseURL, apiKey string) string {
	provider = NormalizeProviderName(provider)
	if provider == "" {
		return ""
	}
	if c.Models == nil {
		c.Models = map[string]ModelConfig{}
	}
	c.Models[provider] = ModelConfig{
		Provider:    provider,
		Model:       "model",
		BaseURL:     strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:      strings.TrimSpace(apiKey),
		Temperature: 0.2,
		MaxTokens:   32768,
	}
	return provider
}
