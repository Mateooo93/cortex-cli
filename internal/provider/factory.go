package provider

import (
	"errors"
	"strings"
)

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
func New(cfg ModelConfig) (Provider, error) {
	apiKey := cfg.APIKey
	baseURL := cfg.BaseURL
	switch strings.ToLower(cfg.Provider) {
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
		if baseURL == "" {
			return nil, errors.New("provider: unsupported provider " + cfg.Provider)
		}
		if apiKey == "" {
			apiKey = "sk-dummy"
		}
		return NewOpenAICompat(strings.ToLower(cfg.Provider), apiKey, baseURL), nil
	}
}
