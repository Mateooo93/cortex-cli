package ui

import (
	"os"
	"sort"
	"strings"

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
}

// AvailableProviders is the static set of providers shown in the left
// column of the Settings tab Model section. Order matters — it's the
// order users see.
var AvailableProviders = []ProviderInfo{
	{Name: "anthropic", DisplayName: "Anthropic"},
	{Name: "openai", DisplayName: "OpenAI"},
	{Name: "openrouter", DisplayName: "OpenRouter"},
	{Name: "opengateway", DisplayName: "OpenGateway"},
	{Name: "minimax", DisplayName: "MiniMax"},
	{Name: "mimo", DisplayName: "Xiaomi MiMo"},
}

// AvailableModels is the curated catalogue of selectable models. OpenRouter
// can route to anything; the entries here are popular routes — users with
// other targets set them via agent frontmatter.
var AvailableModels = []ModelInfo{
	// Anthropic
	{Spec: "anthropic/claude-opus-4-8", Provider: "anthropic", DisplayName: "Claude Opus 4.8"},
	{Spec: "anthropic/claude-opus-4-7", Provider: "anthropic", DisplayName: "Claude Opus 4.7"},
	{Spec: "anthropic/claude-opus-4-6", Provider: "anthropic", DisplayName: "Claude Opus 4.6"},
	{Spec: "anthropic/claude-opus-4-5", Provider: "anthropic", DisplayName: "Claude Opus 4.5"},
	{Spec: "anthropic/claude-sonnet-4-6", Provider: "anthropic", DisplayName: "Claude Sonnet 4.6"},
	{Spec: "anthropic/claude-sonnet-4-5", Provider: "anthropic", DisplayName: "Claude Sonnet 4.5"},
	{Spec: "anthropic/claude-haiku-4-6", Provider: "anthropic", DisplayName: "Claude Haiku 4.6"},
	{Spec: "anthropic/claude-haiku-4-5", Provider: "anthropic", DisplayName: "Claude Haiku 4.5"},
	{Spec: "anthropic/claude-opus-4-0", Provider: "anthropic", DisplayName: "Claude Opus 4.0"},
	{Spec: "anthropic/claude-sonnet-4-0", Provider: "anthropic", DisplayName: "Claude Sonnet 4.0"},
	// OpenAI
	{Spec: "openai/gpt-5.1", Provider: "openai", DisplayName: "GPT-5.1"},
	{Spec: "openai/gpt-5-thinking", Provider: "openai", DisplayName: "GPT-5 Thinking"},
	{Spec: "openai/o3", Provider: "openai", DisplayName: "o3"},
	{Spec: "openai/o4-mini", Provider: "openai", DisplayName: "o4 Mini"},
	{Spec: "openai/gpt-4o", Provider: "openai", DisplayName: "GPT-4o"},
	{Spec: "openai/gpt-4o-mini", Provider: "openai", DisplayName: "GPT-4o Mini"},
	// OpenRouter — curated popular routes; arbitrary slugs go via agent frontmatter.
	{Spec: "openrouter/anthropic/claude-opus-4-8", Provider: "openrouter", DisplayName: "Claude Opus 4.8 (via OpenRouter)"},
	{Spec: "openrouter/anthropic/claude-sonnet-4-6", Provider: "openrouter", DisplayName: "Claude Sonnet 4.6 (via OpenRouter)"},
	{Spec: "openrouter/openai/gpt-5.1", Provider: "openrouter", DisplayName: "GPT-5.1 (via OpenRouter)"},
	{Spec: "openrouter/openai/o3", Provider: "openrouter", DisplayName: "o3 (via OpenRouter)"},
	{Spec: "openrouter/google/gemini-2-flash", Provider: "openrouter", DisplayName: "Gemini 2 Flash (via OpenRouter)"},
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
		envVar := cortexconfig.ProviderEnvVar(p.Name)
		key := pc.APIKey
		if key == "" && envVar != "" {
			key = os.Getenv(envVar)
		}
		prefix := ""
		if key != "" {
			prefix = key
			if len(prefix) > 10 {
				prefix = prefix[:10]
			}
		}
		rows = append(rows, ProviderSettingsView{
			Provider:    p.Name,
			DisplayName: p.DisplayName,
			BaseURL:     baseURL,
			KeyPrefix:   prefix,
			EnvVar:      envVar,
			NeedsAPIKey: cortexconfig.ProviderNeedsAPIKey(p.Name),
		})
	}
	return rows
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
