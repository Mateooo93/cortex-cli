package protocol

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
)

type modelPricing struct {
	prefix     string
	input      float64 // per MTok
	output     float64
	cacheWrite float64
	cacheRead  float64
}

//go:embed model_prices.json
var modelPricesJSON []byte

var pricingByProvider map[string][]modelPricing

func init() {
	pricingByProvider = loadModelPrices()
}

type jsonModelPricing struct {
	Prefix     string  `json:"prefix"`
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheWrite float64 `json:"cache_write"`
	CacheRead  float64 `json:"cache_read"`
}

func loadModelPrices() map[string][]modelPricing {
	var raw map[string][]jsonModelPricing
	if err := json.Unmarshal(modelPricesJSON, &raw); err != nil {
		return map[string][]modelPricing{}
	}
	out := make(map[string][]modelPricing, len(raw))
	for provider, rows := range raw {
		for _, r := range rows {
			out[provider] = append(out[provider], modelPricing{
				prefix:     r.Prefix,
				input:      r.Input,
				output:     r.Output,
				cacheWrite: r.CacheWrite,
				cacheRead:  r.CacheRead,
			})
		}
		sort.Slice(out[provider], func(i, j int) bool {
			return len(out[provider][i].prefix) > len(out[provider][j].prefix)
		})
	}
	return out
}

// ResolvePricingSpec turns a config key or partial name into a provider/model
// spec suitable for CalculateCost. Examples:
//
//	"openai" + provider openai model gpt-5.5 → "openai/gpt-5.5"
//	"anthropic/claude-opus-4-8" → unchanged
func ResolvePricingSpec(model string, provider, bareModel string) string {
	model = strings.TrimSpace(model)
	if strings.Contains(model, "/") {
		return model
	}
	if provider != "" && bareModel != "" {
		return provider + "/" + bareModel
	}
	if provider != "" {
		return provider + "/" + model
	}
	return model
}

// CalculateCost returns the estimated dollar cost for the given token usage.
// model is the full prefixed spec (e.g. "anthropic/claude-opus-4-8"); the
// prefix is stripped before matching the per-provider pricing table.
// Returns 0 when no per-provider table matches.
//
// The API's input_tokens includes cache_creation + cache_read, so we
// subtract those to get the uncached input tokens billed at the regular rate.
func CalculateCost(model string, input, output, cacheWrite, cacheRead int64) float64 {
	provider, bare := splitModelSpec(model)
	table, ok := pricingByProvider[provider]
	if !ok {
		return 0
	}

	var p *modelPricing
	var fallback *modelPricing
	for i := range table {
		if table[i].prefix == "" {
			fallback = &table[i]
			continue
		}
		if strings.HasPrefix(bare, table[i].prefix) {
			p = &table[i]
			break
		}
	}
	if p == nil {
		p = fallback
	}
	if p == nil {
		return 0
	}

	uncachedInput := input - cacheWrite - cacheRead
	if uncachedInput < 0 {
		uncachedInput = 0
	}

	cost := (float64(uncachedInput)*p.input +
		float64(output)*p.output +
		float64(cacheWrite)*p.cacheWrite +
		float64(cacheRead)*p.cacheRead) / 1_000_000

	return cost
}

// splitModelSpec returns (provider, bareModelName) from a prefixed model spec.
func splitModelSpec(spec string) (provider, bare string) {
	if i := strings.Index(spec, "/"); i > 0 {
		return spec[:i], spec[i+1:]
	}
	lower := strings.ToLower(spec)
	switch {
	case strings.HasPrefix(lower, "claude-"):
		return "anthropic", spec
	case strings.HasPrefix(lower, "gpt-"), strings.HasPrefix(lower, "o3"), strings.HasPrefix(lower, "o4"):
		return "openai", spec
	case strings.HasPrefix(lower, "gemini-"):
		return "google", spec
	case strings.HasPrefix(lower, "grok-"):
		return "xai", spec
	case strings.HasPrefix(lower, "deepseek-"):
		return "deepseek", spec
	case strings.HasPrefix(lower, "mistral-"), strings.HasPrefix(lower, "codestral"):
		return "mistral", spec
	case strings.HasPrefix(lower, "llama-"), strings.HasPrefix(lower, "mixtral-"):
		return "groq", spec
	case strings.HasPrefix(lower, "qwen"):
		return "qwen", spec
	case strings.HasPrefix(lower, "minimax"):
		return "minimax", spec
	case strings.HasPrefix(lower, "mimo"):
		return "mimo", spec
	}
	return spec, spec
}