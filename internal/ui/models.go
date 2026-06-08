package ui

import (
	"os"
	"sort"
	"strings"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// ModelInfo describes a single LLM model available for selection in the
// Settings tab. Spec is the prefixed identifier that gets sent on
// session.set_model — the picker never sends a bare model name.
type ModelInfo struct {
	Spec        string // full prefixed identifier, e.g. "anthropic/claude-opus-4-8"
	Provider    string // "anthropic" | "openai" | "openrouter" | "minimax" | "mimo"
	DisplayName string // human-readable label shown in the UI
}

// ProviderInfo describes one provider for the Settings tab provider column.
type ProviderInfo struct {
	Name        string // matches ModelInfo.Provider; also config.ProviderKey.Provider
	DisplayName string // human-readable label shown in the UI
}

// ProviderSettingsView is the rendered API/base-URL state for one provider row.
type ProviderSettingsView struct {
	Provider    string
	DisplayName string
	BaseURL     string
	KeyPrefix   string
	EnvVar      string
	NeedsAPIKey bool
	AuthKind    string // "oauth" | "apikey" | "none" | "env"
	HelpURL     string
	// AuthLabel is the user-facing badge for AuthKind. Set in
	// ProviderSettingsRows: "OAuth (subscription)" for OAuth,
	// "API key" for apikey, "env" for env, "no key" for none.
	AuthLabel string
}

// AvailableProviders is the static fallback list. The Settings tab
// now reads providers from the live config (ProviderNames()), so
// this slice is only used when no config is loaded yet.
var AvailableProviders = []ProviderInfo{
	{Name: "codex", DisplayName: "ChatGPT (codex)"},
	{Name: "xai-sub", DisplayName: "xAI Grok (SuperGrok)"},
	{Name: "claude-sub", DisplayName: "Claude (Pro/Max)"},
	{Name: "openai", DisplayName: "OpenAI"},
	{Name: "anthropic", DisplayName: "Anthropic"},
	{Name: "gemini", DisplayName: "Google Gemini"},
	{Name: "xai", DisplayName: "xAI (Grok)"},
	{Name: "deepseek", DisplayName: "DeepSeek"},
	{Name: "mistral", DisplayName: "Mistral AI"},
	{Name: "groq", DisplayName: "Groq"},
	{Name: "cohere", DisplayName: "Cohere"},
	{Name: "perplexity", DisplayName: "Perplexity"},
	{Name: "openrouter", DisplayName: "OpenRouter"},
	{Name: "opengateway", DisplayName: "OpenGateway"},
	{Name: "minimax", DisplayName: "MiniMax"},
	{Name: "mimo", DisplayName: "Xiaomi MiMo"},
	{Name: "bedrock", DisplayName: "AWS Bedrock"},
	{Name: "cortex", DisplayName: "Cortex"},
	{Name: "ollama", DisplayName: "Ollama"},
	{Name: "lmstudio", DisplayName: "LM Studio"},
	{Name: "vllm", DisplayName: "vLLM"},
	{Name: "copilot", DisplayName: "GitHub Copilot"},
}

// AvailableModels is the curated catalogue of selectable models. OpenRouter
// can route to anything; the entries here are popular routes — users with
// other targets set them via agent frontmatter.
var AvailableModels = []ModelInfo{
	// Anthropic (current as of June 2026)
	{Spec: "anthropic/claude-opus-4-8", Provider: "anthropic", DisplayName: "Claude Opus 4.8"},
	{Spec: "anthropic/claude-opus-4-7", Provider: "anthropic", DisplayName: "Claude Opus 4.7"},
	{Spec: "anthropic/claude-opus-4-6", Provider: "anthropic", DisplayName: "Claude Opus 4.6"},
	{Spec: "anthropic/claude-sonnet-4-6", Provider: "anthropic", DisplayName: "Claude Sonnet 4.6"},
	{Spec: "anthropic/claude-sonnet-4-5", Provider: "anthropic", DisplayName: "Claude Sonnet 4.5"},
	{Spec: "anthropic/claude-haiku-4-6", Provider: "anthropic", DisplayName: "Claude Haiku 4.6"},
	// OpenAI API (current as of June 2026)
	{Spec: "openai/gpt-5.5", Provider: "openai", DisplayName: "GPT-5.5"},
	{Spec: "openai/gpt-5.5-instant", Provider: "openai", DisplayName: "GPT-5.5 Instant"},
	{Spec: "openai/gpt-5.4", Provider: "openai", DisplayName: "GPT-5.4 (computer use)"},
	{Spec: "openai/gpt-5", Provider: "openai", DisplayName: "GPT-5"},
	{Spec: "openai/gpt-5-thinking", Provider: "openai", DisplayName: "GPT-5 Thinking"},
	{Spec: "openai/gpt-5.3-codex", Provider: "openai", DisplayName: "GPT-5.3 Codex"},
	{Spec: "openai/gpt-5-codex", Provider: "openai", DisplayName: "GPT-5 Codex"},
	{Spec: "openai/o3", Provider: "openai", DisplayName: "o3"},
	{Spec: "openai/o4-mini", Provider: "openai", DisplayName: "o4 Mini"},
	{Spec: "openai/gpt-4o", Provider: "openai", DisplayName: "GPT-4o"},
	// ChatGPT (codex) — same models, but authenticated via the
	// user's ChatGPT subscription. Routed through the codex provider,
	// not OpenAI's paid API. Display names note "(ChatGPT)" so users
	// can tell which auth path each row uses.
	{Spec: "codex/gpt-5.5", Provider: "codex", DisplayName: "GPT-5.5 (ChatGPT)"},
	{Spec: "codex/gpt-5.5-instant", Provider: "codex", DisplayName: "GPT-5.5 Instant (ChatGPT)"},
	{Spec: "codex/gpt-5.4", Provider: "codex", DisplayName: "GPT-5.4 (ChatGPT)"},
	{Spec: "codex/gpt-5.3-codex", Provider: "codex", DisplayName: "GPT-5.3 Codex (ChatGPT)"},
	{Spec: "codex/gpt-5-codex", Provider: "codex", DisplayName: "GPT-5 Codex (ChatGPT)"},
	{Spec: "codex/o3", Provider: "codex", DisplayName: "o3 (ChatGPT)"},
	{Spec: "codex/o4-mini", Provider: "codex", DisplayName: "o4 Mini (ChatGPT)"},
	// OpenRouter — curated popular routes; arbitrary slugs go via agent frontmatter.
	{Spec: "openrouter/anthropic/claude-opus-4-8", Provider: "openrouter", DisplayName: "Claude Opus 4.8 (via OpenRouter)"},
	{Spec: "openrouter/anthropic/claude-sonnet-4-6", Provider: "openrouter", DisplayName: "Claude Sonnet 4.6 (via OpenRouter)"},
	{Spec: "openrouter/openai/gpt-5.5", Provider: "openrouter", DisplayName: "GPT-5.5 (via OpenRouter)"},
	{Spec: "openrouter/openai/gpt-5.4", Provider: "openrouter", DisplayName: "GPT-5.4 (via OpenRouter)"},
	{Spec: "openrouter/openai/gpt-5.3-codex", Provider: "openrouter", DisplayName: "GPT-5.3 Codex (via OpenRouter)"},
	{Spec: "openrouter/openai/o3", Provider: "openrouter", DisplayName: "o3 (via OpenRouter)"},
	{Spec: "openrouter/google/gemini-2.5-pro", Provider: "openrouter", DisplayName: "Gemini 2.5 Pro (via OpenRouter)"},
	// OpenGateway — provider-scoped routes; keep the upstream prefix in the model ID.
	{Spec: "opengateway/minimax/minimax-m3", Provider: "opengateway", DisplayName: "MiniMax M3 (via OpenGateway)"},
	{Spec: "opengateway/xiaomi/mimo-v2.5-pro", Provider: "opengateway", DisplayName: "MiMo v2.5 Pro (via OpenGateway)"},
	{Spec: "opengateway/google/gemini-3.1-flash-lite", Provider: "opengateway", DisplayName: "Gemini 3.1 Flash Lite (via OpenGateway)"},
	// MiniMax
	{Spec: "minimax/MiniMax-M2.7", Provider: "minimax", DisplayName: "MiniMax M2.7"},
	{Spec: "minimax/MiniMax-M2.7-highspeed", Provider: "minimax", DisplayName: "MiniMax M2.7 (highspeed)"},
	{Spec: "minimax/MiniMax-M2.5", Provider: "minimax", DisplayName: "MiniMax M2.5"},
	// Xiaomi MiMo
	{Spec: "mimo/mimo-v2.5-pro", Provider: "mimo", DisplayName: "MiMo v2.5 Pro"},
	{Spec: "mimo/mimo-v2.5", Provider: "mimo", DisplayName: "MiMo v2.5"},
	{Spec: "mimo/mimo-v2-flash", Provider: "mimo", DisplayName: "MiMo v2 Flash"},
	// AWS Bedrock (OpenAI-compat Mantle endpoint)
	{Spec: "bedrock/anthropic.claude-opus-4-8", Provider: "bedrock", DisplayName: "Claude Opus 4.8 (via Bedrock)"},
	{Spec: "bedrock/anthropic.claude-sonnet-4-6", Provider: "bedrock", DisplayName: "Claude Sonnet 4.6 (via Bedrock)"},
	{Spec: "bedrock/amazon.nova-pro-v1", Provider: "bedrock", DisplayName: "Amazon Nova Pro (via Bedrock)"},
	{Spec: "bedrock/meta.llama3-3-70b-instruct-v1", Provider: "bedrock", DisplayName: "Llama 3.3 70B (via Bedrock)"},
	// Google Gemini (OpenAI-compat endpoint)
	{Spec: "gemini/gemini-2.5-pro", Provider: "gemini", DisplayName: "Gemini 2.5 Pro"},
	{Spec: "gemini/gemini-2.5-flash", Provider: "gemini", DisplayName: "Gemini 2.5 Flash"},
	{Spec: "gemini/gemini-3.1-pro-preview", Provider: "gemini", DisplayName: "Gemini 3.1 Pro Preview"},
	// xAI (Grok) — paid API key from console.x.ai (not a subscription)
	{Spec: "xai/grok-4", Provider: "xai", DisplayName: "Grok 4 (API key)"},
	{Spec: "xai/grok-4-fast", Provider: "xai", DisplayName: "Grok 4 Fast (API key)"},
	{Spec: "xai/grok-3", Provider: "xai", DisplayName: "Grok 3 (API key)"},
	// xAI Grok (SuperGrok) — OAuth via accounts.x.ai (same flow as Grok Build)
	{Spec: "xai-sub/grok-4.3", Provider: "xai-sub", DisplayName: "Grok 4.3 (SuperGrok)"},
	{Spec: "xai-sub/grok-4", Provider: "xai-sub", DisplayName: "Grok 4 (SuperGrok)"},
	{Spec: "xai-sub/grok-4-fast", Provider: "xai-sub", DisplayName: "Grok 4 Fast (SuperGrok)"},
	{Spec: "xai-sub/grok-build", Provider: "xai-sub", DisplayName: "Grok Build (SuperGrok)"},
	// DeepSeek
	{Spec: "deepseek/deepseek-chat", Provider: "deepseek", DisplayName: "DeepSeek-V3 Chat"},
	{Spec: "deepseek/deepseek-reasoner", Provider: "deepseek", DisplayName: "DeepSeek-R1 Reasoner"},
	// Mistral
	{Spec: "mistral/mistral-large-latest", Provider: "mistral", DisplayName: "Mistral Large"},
	{Spec: "mistral/mistral-medium-latest", Provider: "mistral", DisplayName: "Mistral Medium"},
	{Spec: "mistral/codestral-latest", Provider: "mistral", DisplayName: "Codestral"},
	// Groq
	{Spec: "groq/llama-3.3-70b-versatile", Provider: "groq", DisplayName: "Llama 3.3 70B (via Groq)"},
	{Spec: "groq/llama-3.1-8b-instant", Provider: "groq", DisplayName: "Llama 3.1 8B (via Groq)"},
	{Spec: "groq/mixtral-8x7b-32768", Provider: "groq", DisplayName: "Mixtral 8x7B (via Groq)"},
	// Cohere
	{Spec: "cohere/command-r-plus", Provider: "cohere", DisplayName: "Command R+"},
	{Spec: "cohere/command-r", Provider: "cohere", DisplayName: "Command R"},
	// Perplexity
	{Spec: "perplexity/sonar-pro", Provider: "perplexity", DisplayName: "Sonar Pro"},
	{Spec: "perplexity/sonar", Provider: "perplexity", DisplayName: "Sonar"},
	// Claude (Pro/Max subscription)
	{Spec: "claude-sub/claude-opus-4-8", Provider: "claude-sub", DisplayName: "Claude Opus 4.8 (Pro/Max)"},
	{Spec: "claude-sub/claude-sonnet-4-6", Provider: "claude-sub", DisplayName: "Claude Sonnet 4.6 (Pro/Max)"},
	{Spec: "claude-sub/claude-haiku-4-6", Provider: "claude-sub", DisplayName: "Claude Haiku 4.6 (Pro/Max)"},
	// GitHub Copilot
	{Spec: "copilot/gpt-5.5", Provider: "copilot", DisplayName: "GPT-5.5 (via Copilot)"},
	{Spec: "copilot/gpt-5", Provider: "copilot", DisplayName: "GPT-5 (via Copilot)"},
	{Spec: "copilot/claude-opus-4-8", Provider: "copilot", DisplayName: "Claude Opus 4.8 (via Copilot)"},
	{Spec: "copilot/o3", Provider: "copilot", DisplayName: "o3 (via Copilot)"},
	// Cortex (local gateway)
	{Spec: "cortex/cortex-code", Provider: "cortex", DisplayName: "Cortex Code"},
	// Ollama
	{Spec: "ollama/qwen3.5", Provider: "ollama", DisplayName: "Qwen 3.5 (local)"},
	{Spec: "ollama/llama3.3", Provider: "ollama", DisplayName: "Llama 3.3 (local)"},
	{Spec: "ollama/gemma4", Provider: "ollama", DisplayName: "Gemma 4 (local)"},
	{Spec: "ollama/deepseek-coder-v2", Provider: "ollama", DisplayName: "DeepSeek Coder V2 (local)"},
	// LM Studio
	{Spec: "lmstudio/qwen2.5-7b-instruct", Provider: "lmstudio", DisplayName: "Qwen 2.5 7B (LM Studio)"},
	{Spec: "lmstudio/llama-3.3-8b-instruct", Provider: "lmstudio", DisplayName: "Llama 3.3 8B (LM Studio)"},
	// vLLM
	{Spec: "vllm/meta-llama/Llama-3.3-70B-Instruct", Provider: "vllm", DisplayName: "Llama 3.3 70B (vLLM)"},
	{Spec: "vllm/Qwen/Qwen3-72B-Instruct", Provider: "vllm", DisplayName: "Qwen 3 72B (vLLM)"},
}

// ModelsForProvider returns the entries in AvailableModels whose Provider
// matches the given provider name, in declaration order. Returns nil for
// an unknown provider.
func ModelsForProvider(provider string) []ModelInfo {
	var out []ModelInfo
	for _, m := range AvailableModels {
		if m.Provider == provider {
			out = append(out, m)
		}
	}
	return out
}

// ProvidersFromConfig returns providers from the Cortex config in Settings order.
func ProvidersFromConfig(cfg *cortexconfig.Config) []ProviderInfo {
	if cfg == nil {
		return AvailableProviders
	}
	cfg.EnsureProviderPresets()
	names := cfg.ProviderNames()
	out := make([]ProviderInfo, 0, len(names))
	for _, name := range names {
		display := cortexconfig.ProviderDisplayName(name)
		if display == "" {
			display = name
		}
		out = append(out, ProviderInfo{Name: name, DisplayName: display})
	}
	return out
}

// ProviderSettingsRows returns editable API-key/base-URL rows for Settings.
func ProviderSettingsRows(cfg *cortexconfig.Config) []ProviderSettingsView {
	providers := ProvidersFromConfig(cfg)
	rows := make([]ProviderSettingsView, 0, len(providers))
	for _, p := range providers {
		pc := cortexconfig.ModelConfig{}
		if cfg != nil {
			pc, _ = cfg.ProviderConfig(p.Name)
		}
		baseURL := pc.BaseURL
		if baseURL == "" {
			baseURL = cortexconfig.DefaultBaseURL(p.Name)
		}
		authKind := cortexconfig.ProviderAuthKind(p.Name)
		envVar := cortexconfig.ProviderEnvVar(p.Name)
		prefix := ""
		if authKind == "oauth" {
			prefix = config.OAuthProviderStatusPrefix(p.Name)
		} else {
			key := pc.APIKey
			if key == "" && envVar != "" {
				key = os.Getenv(envVar)
			}
			if key != "" {
				prefix = key
				if len(prefix) > 10 {
					prefix = prefix[:10]
				}
			}
		}
		rows = append(rows, ProviderSettingsView{
			Provider:    p.Name,
			DisplayName: p.DisplayName,
			BaseURL:     baseURL,
			KeyPrefix:   prefix,
			EnvVar:      envVar,
			NeedsAPIKey: cortexconfig.ProviderNeedsAPIKey(p.Name),
			AuthKind:    authKind,
			AuthLabel:   authLabel(authKind),
			HelpURL:     cortexconfig.ProviderHelpURL(p.Name),
		})
	}
	return rows
}

// authLabel returns the user-facing badge text for an auth kind.
func authLabel(kind string) string {
	switch kind {
	case "oauth":
		return "OAuth (subscription)"
	case "apikey":
		return "API key"
	case "env":
		return "env var"
	case "none":
		return "no key"
	default:
		return "API key"
	}
}

func normalizedModelForProvider(providerName, model string) string {
	providerName = cortexconfig.NormalizeProviderName(providerName)
	model = strings.TrimSpace(model)
	if providerName == "opengateway" && !strings.Contains(model, "/") {
		if scope := inferOpenGatewayScopeFromModel(model); scope != "" {
			return scope + "/" + model
		}
	}
	return model
}

func inferOpenGatewayScopeFromModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(model, "minimax"):
		return "minimax"
	case strings.Contains(model, "mimo"):
		return "xiaomi"
	case strings.Contains(model, "gemini"):
		return "google"
	case strings.Contains(model, "qwen"):
		return "qwen"
	case strings.Contains(model, "nemotron") || strings.Contains(model, "nvidia"):
		return "nvidia"
	default:
		return ""
	}
}

func normalizeSettingsModelSpec(spec string, cfg *cortexconfig.Config) string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ""
	}
	if cfg != nil {
		if mc, ok := cfg.Models[spec]; ok {
			if mc.Provider == "" {
				mc.Provider = ProviderOf(spec)
			}
			return cortexconfig.ModelSpec(mc.Provider, normalizedModelForProvider(mc.Provider, mc.Model))
		}
	}
	if providerName, model, ok := cortexconfig.SplitModelSpec(spec); ok {
		return cortexconfig.ModelSpec(providerName, normalizedModelForProvider(providerName, model))
	}
	return spec
}

// ModelsForProviderFromConfig combines configured/fetched models with the
// curated fallback catalogue for a provider. Configured rows are shown first so
// /v1/models refreshes appear immediately in Settings.
func ModelsForProviderFromConfig(providerName string, cfg *cortexconfig.Config) []ModelInfo {
	providerName = cortexconfig.NormalizeProviderName(providerName)
	if providerName == "" {
		return nil
	}
	seenModel := map[string]bool{}
	seenSpec := map[string]bool{}
	var out []ModelInfo

	add := func(spec, model string) {
		spec = strings.TrimSpace(spec)
		model = normalizedModelForProvider(providerName, model)
		if spec == "" || model == "" {
			return
		}
		if normalizedSpec := normalizeSettingsModelSpec(spec, cfg); normalizedSpec != "" {
			spec = normalizedSpec
		}
		if seenSpec[spec] || seenModel[model] {
			return
		}
		out = append(out, ModelInfo{Spec: spec, Provider: providerName, DisplayName: model})
		seenSpec[spec] = true
		seenModel[model] = true
	}

	if cfg != nil {
		cfg.EnsureProviderPresets()
		if mc, ok := cfg.Models[providerName]; ok && cortexconfig.NormalizeProviderName(mc.Provider) == providerName {
			add(cortexconfig.ModelSpec(providerName, mc.Model), mc.Model)
		}
		type row struct {
			key string
			mc  cortexconfig.ModelConfig
		}
		var rows []row
		for key, mc := range cfg.Models {
			if key == providerName {
				continue
			}
			if cortexconfig.NormalizeProviderName(mc.Provider) == providerName {
				rows = append(rows, row{key: key, mc: mc})
			}
		}
		sort.Slice(rows, func(i, j int) bool {
			return strings.ToLower(rows[i].mc.Model) < strings.ToLower(rows[j].mc.Model)
		})
		for _, r := range rows {
			add(cortexconfig.ModelSpec(providerName, r.mc.Model), r.mc.Model)
		}
	}

	for _, m := range AvailableModels {
		if m.Provider != providerName {
			continue
		}
		_, model, ok := cortexconfig.SplitModelSpec(m.Spec)
		if !ok {
			model = m.DisplayName
		}
		if seenSpec[m.Spec] || seenModel[model] {
			continue
		}
		out = append(out, m)
		seenSpec[m.Spec] = true
		seenModel[model] = true
	}
	return out
}

// ProviderOf returns the provider name embedded in a model spec. For
// "openrouter/anthropic/claude-..." the answer is "openrouter" — the
// provider WE talk to, not the upstream routed-through service. Returns
// "" when the spec has no prefix.
func ProviderOf(spec string) string {
	i := strings.Index(spec, "/")
	if i <= 0 {
		return ""
	}
	return spec[:i]
}

// ProviderOfFromConfig resolves the provider for both prefixed specs and bare
// config keys such as "openai" or custom provider keys.
func ProviderOfFromConfig(spec string, cfg *cortexconfig.Config) string {
	if cfg != nil {
		if mc, ok := cfg.Models[spec]; ok {
			return cortexconfig.NormalizeProviderName(mc.Provider)
		}
	}
	if providerName, _, ok := cortexconfig.SplitModelSpec(spec); ok {
		return cortexconfig.NormalizeProviderName(providerName)
	}
	return ProviderOf(spec)
}

// locateActiveModelFromConfig returns Settings cursor coordinates for a model
// using the current Cortex config, including custom providers and fetched rows.
func locateActiveModelFromConfig(spec string, cfg *cortexconfig.Config) (providerIdx, modelIdx int) {
	providers := ProvidersFromConfig(cfg)
	if len(providers) == 0 {
		return 0, 0
	}
	targetSpec := normalizeSettingsModelSpec(spec, cfg)
	for pi, p := range providers {
		models := ModelsForProviderFromConfig(p.Name, cfg)
		for mi, mod := range models {
			if mod.Spec == targetSpec {
				return pi, mi
			}
		}
	}
	specProv := ProviderOfFromConfig(spec, cfg)
	for pi, p := range providers {
		if p.Name == specProv {
			return pi, 0
		}
	}
	return 0, 0
}

// locateActiveModel returns the (providerSel, modelSel) cursor coordinates
// for spec in the two-column Settings picker. When spec isn't in the
// curated catalogue, returns (providerIdxOfSpecPrefix, 0) so the cursor
// lands on the right provider even when the exact model isn't shown
// (e.g. user-installed agent uses a non-curated OpenRouter route).
// Falls back to (0, 0) when even the prefix doesn't match a known provider.
func locateActiveModel(spec string) (providerIdx, modelIdx int) {
	for pi, p := range AvailableProviders {
		models := ModelsForProvider(p.Name)
		for mi, mod := range models {
			if mod.Spec == spec {
				return pi, mi
			}
		}
	}
	// No exact match — fall back to the provider prefix at least.
	specProv := ProviderOf(spec)
	for pi, p := range AvailableProviders {
		if p.Name == specProv {
			return pi, 0
		}
	}
	return 0, 0
}
