package protocol

import "strings"

type modelPricing struct {
	prefix     string
	input      float64 // per MTok
	output     float64
	cacheWrite float64 // 0 means no cache_creation surcharge
	cacheRead  float64
}

// Pricing tables per provider, longest prefix first within each provider so
// short prefixes don't shadow specific model variants.
//
// last updated: 2026-06-06
var pricingByProvider = map[string][]modelPricing{
	"anthropic": {
		// Specific dated variants would go above the generic prefixes; today
		// the agent only sees the canonical names so the generics are fine.
		{"claude-opus-4-8", 5.00, 25.00, 6.25, 0.50},
		{"claude-opus-4-7", 5.00, 25.00, 6.25, 0.50},
		{"claude-opus-4-6", 5.00, 25.00, 6.25, 0.50},
		{"claude-opus-4-5", 5.00, 25.00, 6.25, 0.50},
		{"claude-opus-4", 15.00, 75.00, 18.75, 1.50},
		{"claude-sonnet-4-6", 3.00, 15.00, 3.75, 0.30},
		{"claude-sonnet-4-5", 3.00, 15.00, 3.75, 0.30},
		{"claude-sonnet-4", 3.00, 15.00, 3.75, 0.30},
		{"claude-haiku-4-6", 1.00, 5.00, 1.25, 0.10},
		{"claude-haiku-4-5", 1.00, 5.00, 1.25, 0.10},
	},
	"openai": {
		// OpenAI has no cache_creation surcharge; cacheWrite=0 everywhere.
		// gpt-5.5 is the current flagship; gpt-5.4 is the more
		// affordable coding/computer-use model.
		{"gpt-5.5", 3.00, 12.00, 0, 0.30},
		{"gpt-5.5-instant", 0.50, 2.00, 0, 0.05},
		{"gpt-5.4", 2.50, 10.00, 0, 0.25},
		{"gpt-5.3-codex", 2.50, 10.00, 0, 0.25},
		{"gpt-5-codex", 2.50, 10.00, 0, 0.25},
		{"gpt-5", 2.50, 10.00, 0, 0.25},
		{"gpt-5-thinking", 2.50, 10.00, 0, 0.25},
		{"o4-mini", 1.10, 4.40, 0, 0.275},
		{"o3-mini", 1.10, 4.40, 0, 0.55},
		{"o3", 2.00, 8.00, 0, 0.50},
		{"gpt-4o-mini", 0.15, 0.60, 0, 0.075},
		{"gpt-4o", 2.50, 10.00, 0, 1.25},
	},
	"codex": {
		// ChatGPT-subscription routes — the API still reports token
		// usage; we attribute the dollar cost as if it were the
		// equivalent paid-tier model.
		{"gpt-5.5", 3.00, 12.00, 0, 0.30},
		{"gpt-5.5-instant", 0.50, 2.00, 0, 0.05},
		{"gpt-5.4", 2.50, 10.00, 0, 0.25},
		{"gpt-5.3-codex", 2.50, 10.00, 0, 0.25},
		{"gpt-5-codex", 2.50, 10.00, 0, 0.25},
		{"o4-mini", 1.10, 4.40, 0, 0.275},
		{"o3", 2.00, 8.00, 0, 0.50},
		{"gpt-4o", 2.50, 10.00, 0, 1.25},
	},
	"claude-sub": {
		// Claude Pro/Max subscription routes. We price as the
		// equivalent paid API model for the cost dashboard; the
		// user's actual billing is subscription-based, not metered.
		{"claude-opus-4-8", 5.00, 25.00, 6.25, 0.50},
		{"claude-sonnet-4-6", 3.00, 15.00, 3.75, 0.30},
		{"claude-haiku-4-6", 1.00, 5.00, 1.25, 0.10},
	},
	"copilot": {
		// GitHub Copilot routes — bill via Copilot subscription,
		// not per-token; cost table is for reference only.
		{"gpt-5.5", 3.00, 12.00, 0, 0.30},
		{"gpt-5", 2.50, 10.00, 0, 0.25},
		{"claude-opus-4-8", 5.00, 25.00, 6.25, 0.50},
		{"o3", 2.00, 8.00, 0, 0.50},
	},
	"gemini": {
		// Google Gemini API (via the OpenAI-compat endpoint).
		// Pricing per Google's public rate card (June 2026).
		{"gemini-2.5-pro", 1.25, 10.00, 0, 0.31},
		{"gemini-2.5-flash", 0.075, 0.30, 0, 0.02},
		{"gemini-3.1-pro-preview", 2.00, 12.00, 0, 0.50},
	},
	"xai": {
		// xAI (Grok) — per the published rate card.
		{"grok-4", 3.00, 15.00, 0, 0.75},
		{"grok-4-fast", 0.20, 0.50, 0, 0.05},
		{"grok-3", 3.00, 15.00, 0, 0.75},
	},
	"deepseek": {
		// DeepSeek — known for very aggressive pricing.
		{"deepseek-chat", 0.27, 1.10, 0, 0.07},
		{"deepseek-reasoner", 0.55, 2.19, 0, 0.14},
	},
	"mistral": {
		// Mistral AI La Plateforme.
		{"mistral-large-latest", 2.00, 6.00, 0, 0.60},
		{"mistral-medium-latest", 0.40, 2.00, 0, 0.10},
		{"codestral-latest", 0.30, 0.90, 0, 0.075},
	},
	"groq": {
		// Groq — token-based, fast inference.
		{"llama-3.3-70b-versatile", 0.59, 0.79, 0, 0.00},
		{"llama-3.1-8b-instant", 0.05, 0.08, 0, 0.00},
		{"mixtral-8x7b-32768", 0.24, 0.24, 0, 0.00},
	},
	"cohere": {
		// Cohere Command R family.
		{"command-r-plus", 2.50, 10.00, 0, 0.30},
		{"command-r", 0.15, 0.60, 0, 0.04},
	},
	"perplexity": {
		// Perplexity Sonar family — search-augmented.
		{"sonar-pro", 3.00, 15.00, 0, 0.30},
		{"sonar", 1.00, 1.00, 0, 0.10},
	},
	"minimax": {
		// International endpoint, USD. cn endpoint is RMB-denominated and
		// returns 0 here — callers display "—" in that case.
		{"MiniMax-M2.7-highspeed", 0.279, 1.20, 0, 0.06},
		{"MiniMax-M2.7", 0.279, 1.20, 0, 0.06},
		{"MiniMax-M2.5", 0.15, 0.60, 0, 0.06},
	},
	"mimo": {
		// Xiaomi MiMo Open Platform, USD. Approximate published rates —
		// confirm in-console. No cache_creation surcharge.
		{"mimo-v2.5-pro", 0.435, 0.87, 0, 0},
		{"mimo-v2.5", 0.14, 0.28, 0, 0},
		{"mimo-v2-flash", 0.07, 0.14, 0, 0},
	},
	// "openrouter" intentionally absent — its CostUSD is server-reported
	// and short-circuits in the caller (see daemon/session.go costFor).
}

// CalculateCost returns the estimated dollar cost for the given token usage.
// model is the full prefixed spec (e.g. "anthropic/claude-opus-4-8"); the
// prefix is stripped before matching the per-provider pricing table.
// Returns 0 when no per-provider table matches — the caller should render
// that as "—" in the UI.
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
	for i := range table {
		if strings.HasPrefix(bare, table[i].prefix) {
			p = &table[i]
			break
		}
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
// Examples:
//
//	"anthropic/claude-opus-4-8"             → ("anthropic", "claude-opus-4-8")
//	"openrouter/anthropic/claude-opus-4-8"  → ("openrouter", "anthropic/claude-opus-4-8")
//	"openai/o3"                             → ("openai", "o3")
//	"claude-sonnet-4-6"                     → ("anthropic", "claude-sonnet-4-6")
//
// The last case keeps legacy bare names mapping to Anthropic so existing
// log files and any non-frontmatter callers still see the right pricing.
func splitModelSpec(spec string) (provider, bare string) {
	if i := strings.Index(spec, "/"); i > 0 {
		return spec[:i], spec[i+1:]
	}
	// Legacy bare claude-* — treat as Anthropic for cost-table lookup.
	if strings.HasPrefix(spec, "claude-") {
		return "anthropic", spec
	}
	return "", spec
}
