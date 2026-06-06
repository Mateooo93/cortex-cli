// Package cortexconfig loads and persists the user-facing cortex-cli config.
// The file is YAML, lives at ~/.cortex/config.yaml, and matches the schema
// of the original TypeScript implementation so users can copy their
// existing config over.
package cortexconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProviderPreset describes a built-in OpenAI-compatible provider preset.
type ProviderPreset struct {
	Name         string
	DisplayName  string
	BaseURL      string
	APIKeyEnvVar string
	DefaultModel string
	NeedsAPIKey  bool
}

// BuiltinProviderPresets is the ordered set shown in Settings.
var BuiltinProviderPresets = []ProviderPreset{
	{Name: "cortex", DisplayName: "Cortex", BaseURL: "http://127.0.0.1:8000/v1", APIKeyEnvVar: "CORTEX_API_KEY", DefaultModel: "cortex-code", NeedsAPIKey: false},
	{Name: "openai", DisplayName: "OpenAI", BaseURL: "https://api.openai.com/v1", APIKeyEnvVar: "OPENAI_API_KEY", DefaultModel: "gpt-5.5", NeedsAPIKey: true},
	{Name: "codex", DisplayName: "ChatGPT (codex)", BaseURL: "https://api.openai.com/v1", APIKeyEnvVar: "CODEX_CODEX_TOKEN", DefaultModel: "gpt-5.5", NeedsAPIKey: false},
	{Name: "anthropic", DisplayName: "Anthropic", BaseURL: "https://api.anthropic.com/v1", APIKeyEnvVar: "ANTHROPIC_API_KEY", DefaultModel: "claude-opus-4-8", NeedsAPIKey: true},
	{Name: "ollama", DisplayName: "Ollama", BaseURL: "http://127.0.0.1:11434/v1", APIKeyEnvVar: "", DefaultModel: "qwen3.5", NeedsAPIKey: false},
	{Name: "openrouter", DisplayName: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", APIKeyEnvVar: "OPENROUTER_API_KEY", DefaultModel: "anthropic/claude-opus-4-8", NeedsAPIKey: true},
	{Name: "opengateway", DisplayName: "OpenGateway", BaseURL: "https://opengateway.gitlawb.com/v1", APIKeyEnvVar: "OPENGATEWAY_API_KEY", DefaultModel: "minimax/minimax-m3", NeedsAPIKey: true},
	{Name: "minimax", DisplayName: "MiniMax", BaseURL: "https://api.minimax.io/v1", APIKeyEnvVar: "MINIMAX_API_KEY", DefaultModel: "MiniMax-M2.7", NeedsAPIKey: true},
	{Name: "mimo", DisplayName: "Xiaomi MiMo", BaseURL: "https://api.xiaomimimo.com/v1", APIKeyEnvVar: "MIMO_API_KEY", DefaultModel: "mimo-v2.5-pro", NeedsAPIKey: true},
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
				MaxTokens:        2048,
				ReasoningEffort:  "auto",
				CortexPromptMode: "minimal",
			},
			"openai": {
				Provider:    "openai",
				Model:       "gpt-5.5",
				BaseURL:     "https://api.openai.com/v1",
				APIKey:      "",
				Temperature: 0.2,
				MaxTokens:   2048,
			},
			"anthropic": {
				Provider:    "anthropic",
				Model:       "claude-opus-4-8",
				BaseURL:     "https://api.anthropic.com/v1",
				APIKey:      "",
				Temperature: 0.2,
				MaxTokens:   2048,
			},
			"ollama": {
				Provider:    "ollama",
				Model:       "qwen3.5",
				BaseURL:     "http://127.0.0.1:11434/v1",
				APIKey:      "ollama",
				Temperature: 0.2,
				MaxTokens:   2048,
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
		Streaming: true,
		ShowUsage: true,
		Theme:     "auto",
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
	cfg.EnsureProviderPresets()
	return cfg, nil
}

// Save writes the config to disk in YAML form.
func Save(cfg *Config) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(), data, 0o644)
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
				MaxTokens:   2048,
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
		mc := ModelConfig{Provider: provider, Model: p.DefaultModel, BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), Temperature: 0.2, MaxTokens: 2048}
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
		mc := ModelConfig{Provider: provider, Model: p.DefaultModel, BaseURL: p.BaseURL, APIKey: strings.TrimSpace(apiKey), Temperature: 0.2, MaxTokens: 2048}
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
		MaxTokens:   2048,
	}
	return provider
}
