package provider

import "strings"

// ReasoningEffortForRequest normalizes a session effort level to a value
// providers accept. "auto", "ultracode", and empty are dropped.
func ReasoningEffortForRequest(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "ultracode":
		return "xhigh"
	case "low", "medium", "high", "xhigh", "minimal", "none":
		return strings.ToLower(strings.TrimSpace(effort))
	default:
		return ""
	}
}

// SupportsReasoningEffort reports whether the active model accepts an
// OpenAI-style reasoning_effort request field. Models that reject the
// parameter (most chat/completion models) return false so the CLI layer
// can keep effort session-scoped without breaking requests.
func SupportsReasoningEffort(providerName, model string) bool {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	model = strings.ToLower(strings.TrimSpace(model))

	// Explicit non-reasoning families — safe to skip the param.
	nonReasoning := []string{
		"gpt-4o", "gpt-4.1", "gpt-4-", "gpt-3", "gpt-3.5",
		"claude-haiku", "claude-sonnet-3", "claude-3-",
		"llama", "llama3", "llama-3", "mistral", "mixtral", "gemma",
		"gemini-1", "gemini-2.0-flash", "gemini-flash",
		"deepseek-chat", "phi-", "codellama", "starcoder",
	}
	for _, needle := range nonReasoning {
		if strings.Contains(model, needle) {
			return false
		}
	}

	reasoning := []string{
		"o1", "o3", "o4", "gpt-5", "codex", "cortex-code",
		"deepseek-r1", "deepseek-reasoner", "qwq", "qwen3", "think",
		"reasoner", "r1-", "-r1",
	}
	for _, needle := range reasoning {
		if strings.Contains(model, needle) {
			return true
		}
	}

	switch providerName {
	case "codex", "cortex":
		return true
	case "openai":
		return strings.HasPrefix(model, "o") || strings.Contains(model, "gpt-5")
	case "anthropic":
		// Anthropic's OpenAI-compat surface does not take reasoning_effort.
		return false
	case "ollama":
		return strings.Contains(model, "r1") || strings.Contains(model, "qwen3") || strings.Contains(model, "think")
	default:
		return false
	}
}

// RequestReasoningEffort maps a session effort level to the provider
// request value when the model supports it; otherwise returns "".
func RequestReasoningEffort(providerName, model, sessionEffort string) string {
	effort := ReasoningEffortForRequest(sessionEffort)
	if effort == "" {
		return ""
	}
	if !SupportsReasoningEffort(providerName, model) {
		return ""
	}
	return effort
}