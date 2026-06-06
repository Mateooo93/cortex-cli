package ui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// ModelPickerEntry is one row in the /model picker overlay.
// DisplayName is the human label (e.g. "GPT-5.5 (ChatGPT)") and
// ProviderLabel is the auth method + provider name shown as a
// secondary line so the user knows which model comes from which
// configured provider.
//
// Example rendering:
//
//   ▸ GPT-5.5 (ChatGPT)            codex · OAuth (subscription)
//     GPT-5.5                       openai · API key
//     Claude Opus 4.8               anthropic · API key
//     Qwen 3.5 (local)              ollama · no key
type ModelPickerEntry struct {
	Spec          string // full prefixed spec, e.g. "codex/gpt-5.5"
	DisplayName   string // human label
	ProviderName  string // raw provider name, e.g. "codex"
	ProviderLabel string // "codex · OAuth (subscription)" — auth-aware
	AuthKind      string // "oauth" | "apikey" | "none" | "env"
}

// ModelPicker is a centered overlay that lets the user pick a
// model from the union of curated presets + their configured
// provider's fetched model list. The user can type to filter.
type ModelPicker struct {
	visible  bool
	query    string
	selected int
	entries  []ModelPickerEntry
}

// NewModelPicker creates an empty picker.
func NewModelPicker() ModelPicker {
	return ModelPicker{}
}

// Open populates the picker with entries from the current config
// and shows it. Pass the cortexCfg so the picker can include
// provider-specific fetched models.
func (p *ModelPicker) Open(cfg interface{ ProviderNames() []string }) {
	p.visible = true
	p.query = ""
	p.selected = 0
	p.entries = buildModelPickerEntries()
}

// Close hides the picker.
func (p *ModelPicker) Close() {
	p.visible = false
}

// IsVisible returns whether the picker is showing.
func (p *ModelPicker) IsVisible() bool {
	return p.visible
}

// Refresh re-applies the filter against the existing entries.
func (p *ModelPicker) Refresh() {
	all := buildModelPickerEntries()
	filtered := filterModelPickerEntries(all, p.query)
	p.entries = filtered
	if p.selected >= len(p.entries) {
		p.selected = max(0, len(p.entries)-1)
	}
}

// SetQuery updates the filter query and re-applies the filter.
func (p *ModelPicker) SetQuery(q string) {
	p.query = q
	p.Refresh()
}

// Query returns the current filter query.
func (p *ModelPicker) Query() string {
	return p.query
}

// MoveUp moves the selection up.
func (p *ModelPicker) MoveUp() {
	if p.selected > 0 {
		p.selected--
	}
}

// MoveDown moves the selection down.
func (p *ModelPicker) MoveDown() {
	if p.selected < len(p.entries)-1 {
		p.selected++
	}
}

// Selected returns the spec of the currently highlighted entry,
// or "" if no entry is selected.
func (p *ModelPicker) Selected() string {
	if len(p.entries) == 0 || p.selected < 0 || p.selected >= len(p.entries) {
		return ""
	}
	return p.entries[p.selected].Spec
}

// buildModelPickerEntries assembles the picker rows. The order is:
//
//  1. Curated catalogue (internal/ui/models.go) — gives us the
//     flagship models and a stable display name.
//  2. Per-provider entries for any custom models the user has
//     configured in cortexconfig — so a user who added their own
//     openrouter/<random-model> still sees it here.
//
// Within each provider, entries are sorted alphabetically so the
// picker is stable across launches.
func buildModelPickerEntries() []ModelPickerEntry {
	var entries []ModelPickerEntry
	seen := map[string]bool{}

	// 1. Curated catalogue.
	for _, m := range AvailableModels {
		if seen[m.Spec] {
			continue
		}
		auth := providerAuthKindByName(m.Provider)
		entries = append(entries, ModelPickerEntry{
			Spec:          m.Spec,
			DisplayName:   m.DisplayName,
			ProviderName:  m.Provider,
			ProviderLabel: formatProviderLabel(m.Provider, auth),
			AuthKind:      auth,
		})
		seen[m.Spec] = true
	}

	// 2. Per-provider configured models from cortexconfig.
	// ModelsForProviderFromConfig merges configured + curated and
	// deduplicates; we re-run that here to make sure custom-config
	// rows (e.g. an openrouter route the user added by hand)
	// surface in the picker too.
	providerNames := []string{
		"openai", "anthropic", "gemini", "xai", "deepseek", "mistral",
		"groq", "cohere", "perplexity", "openrouter", "opengateway",
		"minimax", "mimo", "bedrock", "cortex", "ollama", "lmstudio",
		"vllm", "codex", "claude-sub", "copilot",
	}
	for _, prov := range providerNames {
		auth := providerAuthKindByName(prov)
		for _, m := range ModelsForProvider(prov) {
			if seen[m.Spec] {
				continue
			}
			entries = append(entries, ModelPickerEntry{
				Spec:          m.Spec,
				DisplayName:   m.DisplayName,
				ProviderName:  prov,
				ProviderLabel: formatProviderLabel(prov, auth),
				AuthKind:      auth,
			})
			seen[m.Spec] = true
		}
	}

	// Sort by provider name, then display name — gives a stable
	// order across launches and surfaces related models
	// together.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].ProviderName != entries[j].ProviderName {
			return entries[i].ProviderName < entries[j].ProviderName
		}
		return entries[i].DisplayName < entries[j].DisplayName
	})
	return entries
}

// providerAuthKindByName is a thin wrapper so buildModelPickerEntries
// doesn't have to import cortexconfig directly. (Avoids import
// cycles if cortexconfig ever needs to import ui.)
func providerAuthKindByName(name string) string {
	// Hard-coded fallback table. Kept in sync with
	// internal/cortexconfig/config.go's BuiltinProviderPresets.
	// The UI-side mapping lets us format the picker without
	// pulling in the full cortexconfig import graph.
	switch name {
	case "codex", "claude-sub", "copilot":
		return "oauth"
	case "ollama", "lmstudio", "vllm", "cortex":
		return "none"
	case "bedrock":
		return "env"
	default:
		return "apikey"
	}
}

// formatProviderLabel returns the secondary line shown next to a
// model in the picker, e.g. "codex · OAuth (subscription)" or
// "openai · API key". The " · " separator is a non-breaking-ish
// dot so the column aligns in the rendering.
func formatProviderLabel(provider, authKind string) string {
	authLabel := ""
	switch authKind {
	case "oauth":
		authLabel = "OAuth (subscription)"
	case "apikey":
		authLabel = "API key"
	case "env":
		authLabel = "env var"
	case "none":
		authLabel = "no key (local)"
	default:
		authLabel = "API key"
	}
	return fmt.Sprintf("%s · %s", provider, authLabel)
}

// filterModelPickerEntries keeps entries that match query in
// either the display name or the provider label. Empty query
// returns all entries.
func filterModelPickerEntries(entries []ModelPickerEntry, query string) []ModelPickerEntry {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return entries
	}
	var out []ModelPickerEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.DisplayName), query) ||
			strings.Contains(strings.ToLower(e.ProviderLabel), query) ||
			strings.Contains(strings.ToLower(e.Spec), query) {
			out = append(out, e)
		}
	}
	return out
}

// View renders the picker. The picker is a centered modal with
// a search box, a column-aligned list of model rows, and a small
// footer with the currently-highlighted spec.
//
// width is the terminal width. maxHeight caps the visible list
// (we show at most 12 rows; the rest scroll).
func (p *ModelPicker) View(width, maxHeight int, styles Styles) string {
	if !p.visible {
		return ""
	}

	const maxRows = 12
	modalWidth := width - 8
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > width {
		modalWidth = width
	}

	// Title
	title := " Pick a model "
	topBorder := lipgloss.NewStyle().Foreground(colorPrimary).Render("╭─"+title+strings.Repeat("─", modalWidth-lipgloss.Width(title)-3)+"╮")
	bottomBorder := lipgloss.NewStyle().Foreground(colorPrimary).Render("╰"+strings.Repeat("─", modalWidth-2)+"╯")

	// Search box
	searchPrompt := " /"
	searchStyle := lipgloss.NewStyle().Foreground(colorAccentWarm).Bold(true)
	if p.query == "" {
		searchStyle = searchStyle.Italic(true).Foreground(colorDim)
	}
	searchLine := searchStyle.Render(searchPrompt) + p.query + "█"
	searchBox := lipgloss.NewStyle().Width(modalWidth - 4).Render(searchLine)

	// Compute column widths.
	displayCol := 0
	providerCol := 0
	for _, e := range p.entries {
		if w := lipgloss.Width(e.DisplayName); w > displayCol {
			displayCol = w
		}
		if w := lipgloss.Width(e.ProviderLabel); w > providerCol {
			providerCol = w
		}
	}
	// Cap so wide provider labels don't blow out the row.
	if providerCol > 32 {
		providerCol = 32
	}

	total := len(p.entries)
	visible := maxRows
	if visible > total {
		visible = total
	}
	if visible > maxHeight-6 {
		visible = maxHeight - 6
	}
	if visible < 1 {
		visible = 1
	}

	// Sliding window.
	startIdx := 0
	if p.selected >= visible {
		startIdx = p.selected - visible + 1
	}
	endIdx := startIdx + visible
	if endIdx > total {
		endIdx = total
		startIdx = max(0, endIdx-visible)
	}

	var rows []string
	for i := startIdx; i < endIdx; i++ {
		e := p.entries[i]
		var row string
		if i == p.selected {
			marker := lipgloss.NewStyle().Foreground(colorAccentWarm).Bold(true).Render("▸")
			nameStr := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Width(displayCol).Render(e.DisplayName)
			providerStr := lipgloss.NewStyle().Foreground(colorAccentCool).Italic(true).Render(e.ProviderLabel)
			row = fmt.Sprintf("%s  %s   %s", marker, nameStr, providerStr)
		} else {
			marker := " "
			nameStr := lipgloss.NewStyle().Foreground(colorAccentCool).Width(displayCol).Render(e.DisplayName)
			providerStr := lipgloss.NewStyle().Foreground(colorDim).Render(e.ProviderLabel)
			row = fmt.Sprintf("%s  %s   %s", marker, nameStr, providerStr)
		}
		rows = append(rows, lipgloss.NewStyle().Width(modalWidth-4).Render(row))
	}
	if total == 0 {
		emptyLine := lipgloss.NewStyle().Foreground(colorDim).Italic(true).Render("  (no models match your filter)")
		rows = append(rows, emptyLine)
	}

	body := strings.Join([]string{searchBox, "", strings.Join(rows, "\n")}, "\n")
	inner := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderTop(false).
		BorderBottom(false).
		BorderForeground(colorPrimary).
		Width(modalWidth).
		Padding(0, 1).
		Render(body)

	// Footer: currently highlighted spec.
	var footerText string
	if sel := p.Selected(); sel != "" {
		footerText = fmt.Sprintf(" spec: %s   ↑↓ navigate · Enter select · Esc cancel · type to filter", sel)
	} else {
		footerText = " ↑↓ navigate · Enter select · Esc cancel · type to filter"
	}
	footer := lipgloss.NewStyle().
		Foreground(colorDim).
		Width(modalWidth).
		Render(footerText)

	return strings.Join([]string{topBorder, inner, bottomBorder, footer}, "\n")
}
