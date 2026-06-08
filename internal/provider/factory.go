package provider

import (
	"context"
	"errors"
	"strings"
)

// customProviders holds a registry of providers implemented in their
// own packages (e.g. internal/provider/codex) that need to be looked
// up by name. They register themselves in an init() to avoid an
// import cycle: provider → codex → provider.
var customProviders = map[string]func(ctx context.Context) (Provider, error){}

// RegisterCustom adds a factory to the custom-provider table.
// Returns true if the name was added, false if it collided with a
// built-in (callers should validate their names).
//
// Exposed (capital R) so subpackages can register themselves; not
// intended for use outside the provider tree.
func RegisterCustom(name string, fn func(ctx context.Context) (Provider, error)) bool {
	return registerCustomProvider(name, fn)
}

func registerCustomProvider(name string, fn func(ctx context.Context) (Provider, error)) bool {
	if name == "" {
		return false
	}
	switch strings.ToLower(name) {
	case "cortex", "openai", "anthropic", "ollama", "openrouter", "minimax", "mimo", "opengateway":
		return false
	}
	customProviders[strings.ToLower(name)] = fn
	return true
}

// newCustomProvider is called by the factory's default branch and
// returns the custom-registered provider, or (nil, nil) if the name
// is not registered.
func newCustomProvider(ctx context.Context, name string) (Provider, error) {
	fn, ok := customProviders[strings.ToLower(name)]
	if !ok {
		return nil, nil
	}
	return fn(ctx)
}

// ModelConfig describes a single model entry from the user's config.
type ModelConfig struct {
	Provider         string
	Model            string
	BaseURL          string
	APIKey           string
	Temperature      float64
	MaxTokens        int
	ReasoningEffort  string
	CortexPromptMode string
}

// New constructs the right Provider for a given model config. Empty API
// key is allowed (Ollama doesn't require one).
//
// For subscription OAuth providers ("codex", "xai-sub"), cfg.APIKey is
// ignored: authentication comes from the OAuth token in the OS keychain
// (see internal/provider/codex and internal/provider/xaisub).
func New(cfg ModelConfig) (Provider, error) {
	apiKey := cfg.APIKey
	baseURL := cfg.BaseURL
	providerName := strings.ToLower(cfg.Provider)
	switch providerName {
	case "cortex":
		if baseURL == "" {
			baseURL = "http://127.0.0.1:8000/v1"
		}
		if apiKey == "" {
			apiKey = "sk-dummy"
		}
		return NewCortexProvider(apiKey, baseURL), nil
	case "openai":
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return NewOpenAICompat("openai", apiKey, baseURL), nil
	case "anthropic":
		// Anthropic isn't natively OpenAI-compatible, but we use the
		// OpenAI-compat shape pointed at their gateway.
		if baseURL == "" {
			baseURL = "https://api.anthropic.com/v1"
		}
		return NewOpenAICompat("anthropic", apiKey, baseURL), nil
	case "ollama":
		if baseURL == "" {
			baseURL = "http://127.0.0.1:11434/v1"
		}
		if apiKey == "" {
			apiKey = "ollama"
		}
		return NewOpenAICompat("ollama", apiKey, baseURL), nil
	case "openrouter":
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1"
		}
		return NewOpenAICompat("openrouter", apiKey, baseURL), nil
	case "minimax":
		if baseURL == "" {
			baseURL = "https://api.minimax.io/v1"
		}
		return NewOpenAICompat("minimax", apiKey, baseURL), nil
	case "mimo":
		if baseURL == "" {
			baseURL = "https://api.xiaomimimo.com/v1"
		}
		return NewOpenAICompat("mimo", apiKey, baseURL), nil
	default:
		// Custom-registered provider (e.g. "codex")?
		if p, err := newCustomProvider(context.Background(), cfg.Provider); err != nil {
			return nil, err
		} else if p != nil {
			return p, nil
		}
		if baseURL == "" {
			return nil, errors.New("provider: unsupported provider " + cfg.Provider)
		}
		if apiKey == "" {
			apiKey = "sk-dummy"
		}
		return NewOpenAICompat(strings.ToLower(cfg.Provider), apiKey, baseURL), nil
	}
}
