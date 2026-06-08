package ui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// LoginPickerEntry is one row in the /login picker overlay.
// Used for OAuth / subscription sign-in flows. API-key providers
// (openai, anthropic, etc.) are not in this list because the user
// already configures them via the right-panel key input.
type LoginPickerEntry struct {
	Provider   string // "codex" | "claude-sub" | "copilot"
	Label      string // human label shown in the picker
	AuthMethod string // "browser" | "device-code" | "env-var"
	Help       string // one-line description of the flow
}

// LoginPicker is a centered modal that lists the OAuth providers
// the user can sign in to. Picking one fires the appropriate flow
// (browser OAuth for codex, env-var status for the others, etc.).
// The user can also type "codex --device" to use the device-code
// fallback when the browser flow is blocked.
type LoginPicker struct {
	visible  bool
	query    string
	selected int
	entries  []LoginPickerEntry
}

// NewLoginPicker creates an empty picker.
func NewLoginPicker() LoginPicker { return LoginPicker{} }

// Open shows the picker. The available entries come from
// BuiltinProviderPresets filtered to auth_kind=oauth.
func (p *LoginPicker) Open() {
	p.visible = true
	p.query = ""
	p.selected = 0
	p.entries = buildLoginPickerEntries()
}

// Close hides the picker.
func (p *LoginPicker) Close()         { p.visible = false }
func (p *LoginPicker) IsVisible() bool { return p.visible }

// SetQuery updates the filter query and re-applies the filter.
func (p *LoginPicker) SetQuery(q string) { p.query = q; p.Refresh() }
func (p *LoginPicker) Query() string     { return p.query }

// Refresh re-applies the filter and clamps the cursor.
func (p *LoginPicker) Refresh() {
	all := buildLoginPickerEntries()
	p.entries = filterLoginPickerEntries(all, p.query)
	if p.selected >= len(p.entries) {
		p.selected = max(0, len(p.entries)-1)
	}
}

// MoveUp / MoveDown navigate.
func (p *LoginPicker) MoveUp() {
	if p.selected > 0 {
		p.selected--
	}
}
func (p *LoginPicker) MoveDown() {
	if p.selected < len(p.entries)-1 {
		p.selected++
	}
}

// Selected returns the (provider, wantDevice) tuple for the
// currently-highlighted entry, or ("", false) if no entry.
func (p *LoginPicker) Selected() (provider string, wantDevice bool) {
	if len(p.entries) == 0 || p.selected < 0 || p.selected >= len(p.entries) {
		return "", false
	}
	// The query string is the source of truth for "--device":
	// typing "codex --device" should route the login to the
	// device-code flow regardless of which row is highlighted.
	q := strings.ToLower(p.query)
	if strings.Contains(q, "device") {
		return p.entries[p.selected].Provider, true
	}
	return p.entries[p.selected].Provider, false
}

// buildLoginPickerEntries assembles the picker rows. We hard-code
// the OAuth provider list rather than reading from
// cortexconfig.BuiltinProviderPresets so the picker can stay
// independent of that package's import cycle (cortexconfig is
// imported by the ui package but we don't want it to import ui).
func buildLoginPickerEntries() []LoginPickerEntry {
	return []LoginPickerEntry{
		{
			Provider:   "codex",
			Label:      "ChatGPT (codex) \u2014 GPT-5.5, o3, etc.",
			AuthMethod: "browser",
			Help:       "Opens auth.openai.com in your browser. Type \"codex --device\" for the device-code fallback.",
		},
		{
			Provider:   "xai-sub",
			Label:      "xAI Grok (SuperGrok) \u2014 Grok Build, Composer 2.5",
			AuthMethod: "browser",
			Help:       "Opens accounts.x.ai in your browser. Uses your SuperGrok or X Premium+ subscription (Grok Build + Composer 2.5).",
		},
		{
			Provider:   "claude-sub",
			Label:      "Claude Pro / Max \u2014 Claude Opus 4.8, etc.",
			AuthMethod: "env-var",
			Help:       "Set CLAUDE_CODE_OAUTH_TOKEN in your environment, then restart cortex-cli.",
		},
		{
			Provider:   "copilot",
			Label:      "GitHub Copilot \u2014 GPT-5.5, Claude 4.8, etc.",
			AuthMethod: "env-var",
			Help:       "Set COPILOT_OAUTH_TOKEN in your environment, then restart cortex-cli.",
		},
	}
}

// filterLoginPickerEntries keeps entries whose Label, Provider,
// or Help text matches the query. Empty query returns all.
func filterLoginPickerEntries(entries []LoginPickerEntry, query string) []LoginPickerEntry {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		// Sort by provider name for stable ordering.
		out := make([]LoginPickerEntry, len(entries))
		copy(out, entries)
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Provider < out[j].Provider
		})
		return out
	}
	var out []LoginPickerEntry
	for _, e := range entries {
		haystack := strings.ToLower(e.Provider + " " + e.Label + " " + e.Help)
		if strings.Contains(haystack, query) {
			out = append(out, e)
		}
	}
	return out
}

// VisibleHeight returns the modal's height for centering.
func (p *LoginPicker) VisibleHeight() int {
	if !p.visible {
		return 0
	}
	// title + sep + rows (cap 8) + blank + footer
	visible := len(p.entries)
	if visible > 8 {
		visible = 8
	}
	return 2 + visible + 2
}

// View renders the picker as a centered modal.
func (p *LoginPicker) View(width, maxHeight int, styles Styles) string {
	if !p.visible {
		return ""
	}
	const maxRows = 8
	modalWidth := width - 8
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > width {
		modalWidth = width
	}

	title := " Sign in to a subscription "
	ts := lipgloss.NewStyle().Foreground(colorPrimary)
	topBorder := ts.Render("\u256d\u2500" + title + strings.Repeat("\u2500", modalWidth-lipgloss.Width(title)-3) + "\u256e")
	bottomBorder := ts.Render("\u2570" + strings.Repeat("\u2500", modalWidth-2) + "\u256f")

	// Search box
	prompt := " /"
	searchStyle := lipgloss.NewStyle().Foreground(colorAccentWarm).Bold(true)
	if p.query == "" {
		searchStyle = searchStyle.Italic(true).Foreground(colorDim)
	}
	searchLine := searchStyle.Render(prompt) + p.query + "\u2588"
	searchBox := lipgloss.NewStyle().Width(modalWidth - 4).Render(searchLine)

	// Column widths
	providerCol := 0
	labelCol := 0
	for _, e := range p.entries {
		if w := lipgloss.Width(e.Provider); w > providerCol {
			providerCol = w
		}
		if w := lipgloss.Width(e.Label); w > labelCol {
			labelCol = w
		}
	}
	if providerCol > 14 {
		providerCol = 14
	}
	if labelCol > modalWidth-26 {
		labelCol = modalWidth - 26
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
			marker := lipgloss.NewStyle().Foreground(colorAccentWarm).Bold(true).Render("\u25b8")
			provStr := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Width(providerCol).Render(e.Provider)
			labelStr := lipgloss.NewStyle().Foreground(colorPrimary).Width(labelCol).Render(e.Label)
			row = fmt.Sprintf("%s  %s  %s", marker, provStr, labelStr)
		} else {
			marker := " "
			provStr := lipgloss.NewStyle().Foreground(colorAccentCool).Width(providerCol).Render(e.Provider)
			labelStr := lipgloss.NewStyle().Foreground(colorDim).Width(labelCol).Render(e.Label)
			row = fmt.Sprintf("%s  %s  %s", marker, provStr, labelStr)
		}
		rows = append(rows, lipgloss.NewStyle().Width(modalWidth-4).Render(row))
	}
	if total == 0 {
		emptyLine := lipgloss.NewStyle().Foreground(colorDim).Italic(true).Render("  (no OAuth providers match your filter)")
		rows = append(rows, emptyLine)
	}

	// Add help text for the highlighted row
	if sel := p.selected; sel >= 0 && sel < len(p.entries) {
		help := p.entries[sel].Help
		helpLine := lipgloss.NewStyle().Foreground(colorDim).Italic(true).Width(modalWidth - 4).Render("  " + help)
		rows = append(rows, helpLine)
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

	footerText := "\u2191\u2193 navigate \u00b7 Enter sign in \u00b7 Esc cancel \u00b7 type \"codex --device\" for device-code"
	footer := lipgloss.NewStyle().
		Foreground(colorDim).
		Width(modalWidth).
		Render(" " + footerText)

	return strings.Join([]string{topBorder, inner, bottomBorder, footer}, "\n")
}
