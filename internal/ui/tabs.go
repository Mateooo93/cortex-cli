package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// TabKind identifies the type of a tab.
type TabKind int

const (
	TabKindSessions TabKind = iota // sessions list overview
	TabKindChat                    // chat display for the selected session
	TabKindSettings                // global settings
)

// formatRunningTime formats a duration as a human-readable running time string.
func formatRunningTime(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", h, m)
}

// waitingBadge is the "Waiting for input" styled tag shown on sessions that need user attention.
var waitingBadge = lipgloss.NewStyle().Background(colorSecondary).Foreground(lipgloss.Color("0")).Bold(true).Render(" Waiting for input ")

// unreadDotStyle styles the ● indicator for sessions with unread messages.
var unreadDotStyle = lipgloss.NewStyle().Foreground(colorSecondary)

// selectedRowStyle is the shared cursor highlight for Sessions and Settings rows.
func selectedRowStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colorPrimary)
}

// renderSessionsView renders the sessions list overview.
func renderSessionsView(sessions []*SessionState, width, height int, s Styles, filter, inputView string, selectedRow int) string {
	const colSession = 10
	const colRunning = 10

	// Sort sessions by creation time, newest at the top.
	// The user reported: "sort sessions by date from
	// newest (top) to oldest (bottom)". m.sessions is in
	// creation order (oldest at index 0, newest at the
	// end), so we copy the slice and sort a stable-sort
	// copy. We don't mutate m.sessions itself because the
	// model still uses the original indices for
	// m.selectedSession / findSessionByDaemonID / etc.
	// — only the visual list needs to be flipped.
	sorted := make([]*SessionState, len(sessions))
	copy(sorted, sessions)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].createdAt.After(sorted[j].createdAt)
	})
	sessions = sorted


	// Help banner: description line + shortcuts line + separator.
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	whiteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	shortcut := func(key, action string) string {
		return keyStyle.Render(key) + " " + dimStyle.Render(action)
	}
	shortcuts := strings.Join([]string{
		shortcut("a", "new"),
		shortcut("x", "close"),
		shortcut("↑↓", "navigate"),
		shortcut("enter", "open"),
		shortcut("type", "filter"),
	}, "   ")
	innerWidth := width - 4 // width outer − 2 border sides − 2 padding sides
	if innerWidth < 0 {
		innerWidth = 0
	}

	// colMessage fills the remaining space: innerWidth minus the two fixed columns,
	// the 6 characters of inter-column padding ("  " × 3 in the header), and the
	// 22-character badge slot ("  " + " Waiting for input ") always reserved so
	// the layout stays stable whether or not any session needs input.
	const badgeVisible = 22 // len("  ") + len(" Waiting for input ")
	colMessage := innerWidth - colSession - colRunning - 6 - badgeVisible
	if colMessage < 20 {
		colMessage = 20
	}
	helpBlock := whiteStyle.Render("Manage your coding sessions across workspaces.") + "\n" +
		shortcuts + "\n" +
		dimStyle.Render(strings.Repeat("─", innerWidth))

	header := fmt.Sprintf("  %-*s  %-*s  %-*s%-*s", colSession, "Session", colMessage, "First message", colRunning, "Running", badgeVisible, "")
	rows := []string{s.TabActiveStyle.Render(header)}

	filterLower := strings.ToLower(filter)
	rowIdx := 0

	for _, sess := range sessions {
		sessionCol := "connecting…"
		runningCol := "—"
		if sess.client != nil {
			id := sess.client.SessionID()
			if dash := strings.Index(id, "-"); dash >= 0 {
				sessionCol = id[:dash]
			} else if len(id) > colSession {
				sessionCol = id[:colSession]
			} else {
				sessionCol = id
			}
			if !sess.client.StartedAt().IsZero() {
				runningCol = formatRunningTime(time.Since(sess.client.StartedAt()))
			}
		} else if sess.label != "" {
			// Restored placeholder — show the saved label so the
			// user can recognise their previous sessions.
			sessionCol = sess.label
			if len(sessionCol) > colSession {
				sessionCol = sessionCol[:colSession]
			}
		} else if sess.persistID != "" {
			// Has a saved ID but no client — show the short ID.
			sessionCol = sess.persistID
			if dash := strings.Index(sessionCol, "-"); dash >= 0 {
				sessionCol = sessionCol[:dash]
			} else if len(sessionCol) > colSession {
				sessionCol = sessionCol[:colSession]
			}
		} else if sess.modelName != "" {
			// Brand-new session that hasn't connected yet.
			sessionCol = sess.modelName
			if len(sessionCol) > colSession {
				sessionCol = sessionCol[:colSession]
			}
		}

		msgCol := "—"
		if sess.parentID != "" {
			parentShort := sess.parentID
			if dash := strings.Index(parentShort, "-"); dash >= 0 {
				parentShort = parentShort[:dash]
			} else if len(parentShort) > 8 {
				parentShort = parentShort[:8]
			}
			prefix := "⎇ " + parentShort + "/" + fmt.Sprintf("%d", sess.forkTurnIdx+1) + "  "
			rest := "—"
			for _, msg := range sess.chatMessages {
				if msg.Type == MsgUser {
					rest = strings.SplitN(msg.Text, "\n", 2)[0]
					break
				}
			}
			if rest == "—" && sess.modelName != "" {
				rest = sess.modelName
			}
			full := prefix + rest
			if len(full) > colMessage {
				full = full[:colMessage-1] + "…"
			}
			msgCol = full
		} else {
			for _, msg := range sess.chatMessages {
				if msg.Type == MsgUser {
					line := strings.SplitN(msg.Text, "\n", 2)[0]
					if len(line) > colMessage {
						line = line[:colMessage-1] + "…"
					}
					msgCol = line
					break
				}
			}
			if msgCol == "—" {
				// No user message yet — show the model name so
				// the user knows what model the session is on.
				if sess.modelName != "" {
					msgCol = sess.modelName
				} else if sess.label != "" {
					msgCol = sess.label
				}
				if len(msgCol) > colMessage {
					msgCol = msgCol[:colMessage-1] + "…"
				}
			}
		}

		if filterLower != "" &&
			!strings.Contains(strings.ToLower(sessionCol), filterLower) &&
			!strings.Contains(strings.ToLower(msgCol), filterLower) {
			continue
		}

		hasUnread := sess.unreadCount > 0
		needsInput := sess.agentState == StateConfirmPending || sess.agentState == StateUserQuestion
		var badgeSlot string
		if needsInput {
			badgeSlot = "  " + waitingBadge
		} else {
			badgeSlot = strings.Repeat(" ", badgeVisible)
		}
		plainCols := fmt.Sprintf("%-*s  %-*s  %-*s", colSession, sessionCol, colMessage, msgCol, colRunning, runningCol) + badgeSlot
		if rowIdx == selectedRow {
			dotChar := " "
			if hasUnread {
				dotChar = "●"
			}
			// Highlight session data only — not column padding to the badge slot.
			selectText := fmt.Sprintf("%s %-*s  %s  %s", dotChar, colSession, sessionCol, msgCol, runningCol)
			line := renderSettingsSelectLine(selectedRowStyle(), selectText, innerWidth)
			if needsInput {
				line += "  " + waitingBadge
			}
			rows = append(rows, line)
		} else if hasUnread {
			rows = append(rows, unreadDotStyle.Render("●")+" "+plainCols)
		} else {
			rows = append(rows, "  "+plainCols)
		}
		rowIdx++
	}

	content := helpBlock + "\n" + inputView + "\n" + strings.Join(rows, "\n")
	return s.ViewportFocusedStyle.Width(width).Height(height).Render(content)
}

// renderSettingsSectionTabs draws Providers | Other Settings with Tab hint.
func renderSettingsSectionTabs(activeSection int, innerWidth int, s Styles) string {
	sep := lipgloss.NewStyle().Foreground(colorDim).Render(" │ ")
	names := []string{"Providers", "Other Settings"}
	var parts []string
	for i, name := range names {
		tab := " " + name + " "
		if i == activeSection {
			parts = append(parts, s.TabActiveStyle.Render(tab))
		} else {
			parts = append(parts, s.TabInactiveStyle.Render(tab))
		}
		if i == 0 {
			parts = append(parts, sep)
		}
	}
	tabs := strings.Join(parts, "")
	hint := lipgloss.NewStyle().Foreground(colorDim).Italic(true).Render("Tab")
	gap := innerWidth - lipgloss.Width(tabs) - lipgloss.Width(hint)
	if gap < 1 {
		gap = 1
	}
	return lipgloss.NewStyle().Width(innerWidth).Render(tabs + strings.Repeat(" ", gap) + hint)
}

func settingsWindow(sel, total, limit int) (int, int) {
	if total <= 0 || limit <= 0 {
		return 0, 0
	}
	if limit > total {
		limit = total
	}
	if sel < 0 {
		sel = 0
	}
	if sel >= total {
		sel = total - 1
	}
	start := sel - limit/2
	if start < 0 {
		start = 0
	}
	if start+limit > total {
		start = total - limit
	}
	return start, start + limit
}

func settingsTruncate(text string, width int) string {
	text = strings.TrimSpace(text)
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

// renderSettingsSelectLine highlights only the row text, not trailing
// padding to the viewport edge.
func renderSettingsSelectLine(style lipgloss.Style, text string, innerWidth int) string {
	return style.Render(settingsTruncate(text, innerWidth))
}

// renderSettingsLine renders a full-width settings row without selection fill.
func renderSettingsLine(style lipgloss.Style, text string, innerWidth int) string {
	return style.Width(innerWidth).Render(settingsTruncate(text, innerWidth))
}

// normalizedSettingsValue turns empty/blank string into "auto" for display.
func normalizedSettingsValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "auto"
	}
	return v
}

func settingsKeyStatus(pk ProviderSettingsView) string {
	if pk.KeyPrefix != "" {
		return "set (" + settingsTruncate(pk.KeyPrefix, 8) + "…)"
	}
	if !pk.NeedsAPIKey {
		return "not required"
	}
	if pk.EnvVar != "" {
		return "missing · " + pk.EnvVar
	}
	return "missing"
}

type SettingsOtherView struct {
	Theme           string
	PrimaryColor    string
	ShowThinking    bool
	ReasoningEffort string
	ShowUsage       bool
	AutoCompact     bool
}

// SettingsInspectView is the rendered state for the inline provider
// detail panel. Unused by the wizard path; kept for callers that still
// surface it.
type SettingsInspectView struct {
	Provider    string
	DisplayName string
	BaseURL     string
	HasAPIKey   bool
	KeyPrefix   string
	Field       int
	NeedsAPIKey bool
	EnvVar      string
	AuthKind    string // "oauth" | "apikey" | "none" | "env"
	HelpURL     string
}

// SettingsWizardFieldView describes one editable row inside the
// provider edit wizard. The view layer never reads from
// settingsWizard directly; everything it needs is pre-computed.
type SettingsWizardFieldView struct {
	Label       string
	Value       string // redacted form for the API key, full form otherwise
	Placeholder string
	Focused     bool
	Editable    bool
	Hint        string // extra context (e.g. "preset — read-only")
}

// SettingsWizardView is the rendered state of the provider edit wizard.
type SettingsWizardView struct {
	Provider    string
	DisplayName string
	IsCustom    bool
	Field       settingsWizardField
	InputView   string
	Fields      []SettingsWizardFieldView
}

// renderSettingsView renders the Settings tab content.
func renderSettingsView(width, height int, s Styles, activeSection, providerSel, modelSel, modelColumn int, activeModel, activeProviderName string, providers []ProviderInfo, selectedModels []ModelInfo, keys []ProviderSettingsView, keySel int, otherSel int, other SettingsOtherView, inspect SettingsInspectView, inKeyInput bool, keyInputLabel, keyInputView string, wizard SettingsWizardView) string {
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	mutedStyle := lipgloss.NewStyle().Foreground(colorDim)
	// selectedStyle is the cursor highlight in the provider
	// list. The user reported the cursor was hard to see; we
	// make it bright + bold + bracketed so the user can tell
	// exactly which row is selected regardless of which
	// provider is currently active.
	selectedStyle := selectedRowStyle()
	activeStyle := lipgloss.NewStyle().Foreground(colorDim)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	innerWidth := width - 4
	if innerWidth < 0 {
		innerWidth = 0
	}
	if inKeyInput {
		activeSection = 0
	}
	if wizard.Provider != "" {
		activeSection = 0
	}
	if activeSection < 0 || activeSection > 1 {
		activeSection = 0
	}

	clampRows := func(v, minV, maxV int) int {
		if v < minV {
			return minV
		}
		if v > maxV {
			return maxV
		}
		return v
	}
	divider := dimStyle.Width(innerWidth).Render(strings.Repeat("─", innerWidth))

	// When the provider edit wizard is open it fills the whole
	// Settings viewport and replaces the Models / Other Settings
	// sections. Render it here so the rest of the normal layout
	// (section switching, provider list, etc.) is suppressed.
	if wizard.Provider != "" {
		return renderSettingsWizardView(width, height, s, dimStyle, selectedStyle, activeStyle, titleStyle, innerWidth, divider, wizard)
	}

	lines := []string{
		titleStyle.Width(innerWidth).Render("Settings"),
		renderSettingsSectionTabs(activeSection, innerWidth, s),
	}

	if inKeyInput {
		lines = append(lines,
			dimStyle.Width(innerWidth).Render(settingsTruncate(keyInputLabel, innerWidth)),
			keyInputView,
			"",
			dimStyle.Italic(true).Width(innerWidth).Render("Enter save · Esc cancel"),
		)
		return s.ViewportFocusedStyle.Width(width).Height(height).Render(strings.Join(lines, "\n"))
	}

	if activeSection == 0 {
	// Providers section: provider name only, no status text.
	apiRowsLimit := clampRows(height/6, 3, 6)
	keyStart, keyEnd := settingsWindow(keySel, len(keys), apiRowsLimit)
	// Provider names are bold and bright white so they read as headings.
	providerNameStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	if keyStart > 0 {
		lines = append(lines, mutedStyle.Width(innerWidth).Render("  ↑ more providers"))
	}
	for i := keyStart; i < keyEnd; i++ {
		pk := keys[i]
		isCursor := inspect.Provider == "" && i == keySel
		prefix := "  "
		if isCursor {
			prefix = "▸ "
		}
		row := prefix + pk.DisplayName
		rowStyle := providerNameStyle
		if isCursor {
			rowStyle = selectedStyle
		}
		if isCursor {
			lines = append(lines, renderSettingsSelectLine(rowStyle, row, innerWidth))
		} else {
			lines = append(lines, renderSettingsLine(rowStyle, row, innerWidth))
		}
	}
	if keyEnd < len(keys) {
		lines = append(lines, mutedStyle.Width(innerWidth).Render(fmt.Sprintf("  ↓ %d more providers", len(keys)-keyEnd)))
	}
	if inspect.Provider != "" {
		// Inline detail panel for the selected provider.
		baseURLValue := inspect.BaseURL
		if baseURLValue == "" {
			baseURLValue = "(not set — will use provider default)"
		}
		keyField := "API key"
		keyValue := "(not set)"
		if inspect.AuthKind == "oauth" {
			keyField = "Sign-in"
			switch {
			case inspect.KeyPrefix != "":
				keyValue = inspect.KeyPrefix
			default:
				keyValue = "(not connected — Enter on provider row opens browser sign-in)"
			}
		} else if inspect.HasAPIKey {
			keyValue = inspect.KeyPrefix + "…"
		} else if !inspect.NeedsAPIKey {
			keyValue = "(not required)"
		}
		fieldLabel := func(idx int, label string) string {
			marker := "  "
			if inspect.Field == idx {
				marker = "▸ "
			}
			return marker + label
		}
		inspectFieldLine := func(idx int, label string) string {
			text := fieldLabel(idx, label)
			if inspect.Field == idx {
				return renderSettingsSelectLine(selectedStyle, text, innerWidth)
			}
			return renderSettingsLine(mutedStyle, text, innerWidth)
		}
		lines = append(lines,
			renderSettingsLine(mutedStyle, "  ── "+inspect.DisplayName+" ──", innerWidth),
			inspectFieldLine(0, "Name"),
			renderSettingsLine(mutedStyle, "    "+inspect.DisplayName, innerWidth),
			inspectFieldLine(1, "Base URL"),
			renderSettingsLine(mutedStyle, "    "+baseURLValue, innerWidth),
			inspectFieldLine(2, keyField),
			renderSettingsLine(mutedStyle, "    "+keyValue, innerWidth),
		)
		// Show the auth kind as a one-line badge so the user knows
		// whether the row expects an API key, an OAuth subscription
		// sign-in, an env var, or nothing at all. This is the single
		// most-confused field in the table; the badge is the
		// quickest way to communicate it.
		authLabel := "API key"
		authHelp := "Paste the API key from the provider's dashboard."
		switch inspect.AuthKind {
		case "oauth":
			authLabel = "OAuth (subscription)"
			authHelp = "Sign in with your existing subscription; no API key needed."
		case "none":
			authLabel = "no key (local server)"
			authHelp = "Local model server. Make sure it's running."
		case "env":
			authLabel = "env var only"
			authHelp = "Read from " + inspect.EnvVar + " — paste the value into your environment."
		}
		lines = append(lines,
			mutedStyle.Width(innerWidth).Render("    auth: "+authLabel),
			dimStyle.Italic(true).Width(innerWidth).Render("    "+authHelp),
		)
		if inspect.HelpURL != "" {
			lines = append(lines, dimStyle.Italic(true).Width(innerWidth).Render("    "+settingsTruncate(inspect.HelpURL, innerWidth)))
		}
	}
	} else {
	thinkingStatus := "Off"
	thinkingToggle := "[ ]"
	if other.ShowThinking {
		thinkingStatus = "On"
		thinkingToggle = "[✓]"
	}
	usageStatus := "Off"
	usageToggle := "[ ]"
	if other.ShowUsage {
		usageStatus = "On"
		usageToggle = "[✓]"
	}
	compactStatus := "Off"
	compactToggle := "[ ]"
	if other.AutoCompact {
		compactStatus = "On"
		compactToggle = "[✓]"
	}
	otherRows := []struct {
		label string
		value string
	}{
		{label: "Theme", value: normalizedSettingsValue(other.Theme)},
		{label: "Primary color", value: other.PrimaryColor},
		{label: "Show extended thinking", value: thinkingToggle + " " + thinkingStatus},
		{label: "Reasoning effort", value: normalizedSettingsValue(other.ReasoningEffort)},
		{label: "Show token usage", value: usageToggle + " " + usageStatus},
		// Auto-compact context: triggers a /compact run when
		// usage exceeds 80% of the model's context window.
		// The slash command `/compact` always works regardless
		// of this setting.
		{label: "Auto-compact context", value: compactToggle + " " + compactStatus},
	}
	for i, row := range otherRows {
		prefix := "  "
		isActive := i == otherSel
		if isActive {
			prefix = "▸ "
		}
		rowText := settingsTruncate(fmt.Sprintf("%s%-26s %s", prefix, row.label, row.value), innerWidth)
		rowStyle := mutedStyle
		if isActive {
			rowStyle = selectedStyle
		}
		if isActive {
			lines = append(lines, renderSettingsSelectLine(rowStyle, rowText, innerWidth))
		} else {
			lines = append(lines, renderSettingsLine(rowStyle, rowText, innerWidth))
		}
	}
	}
	content := strings.Join(lines, "\n")
	return s.ViewportFocusedStyle.Width(width).Height(height).Render(content)
}

// renderSettingsWizardView renders the full-section provider edit
// wizard. The wizard fills the entire Settings viewport; the regular
// Models and Other Settings sections are hidden while it is open. The
// three fields are stacked vertically with the active row highlighted
// and a single text input bound to the active field sits at the bottom
// so the user can edit without a separate pop-up.
func renderSettingsWizardView(width, height int, s Styles, dimStyle, selectedStyle, activeStyle, titleStyle lipgloss.Style, innerWidth int, divider string, w SettingsWizardView) string {
	whiteStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	fieldLabelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))

	lines := []string{
		titleStyle.Width(innerWidth).Render("Settings"),
		renderSettingsSectionTabs(0, innerWidth, s),
	}

	// Heading showing the provider being edited.
	displayName := w.DisplayName
	if displayName == "" {
		displayName = w.Provider
	}
	subtitle := "Editing: " + displayName
	if w.IsCustom {
		subtitle += "  (custom)"
	}
	lines = append(lines,
		whiteStyle.Width(innerWidth).Render(settingsTruncate(subtitle, innerWidth)),
		divider,
	)

	fieldRows := []struct {
		idx    settingsWizardField
		view   SettingsWizardFieldView
		detail string
	}{
		{wizardFieldName, w.Fields[0], ""},
		{wizardFieldBaseURL, w.Fields[1], ""},
		{wizardFieldAPIKey, w.Fields[2], ""},
	}
	if len(w.Fields) < 3 {
		// Defensive: pad missing fields rather than panic.
		for len(w.Fields) < 3 {
			w.Fields = append(w.Fields, SettingsWizardFieldView{})
		}
		fieldRows[0].view = w.Fields[0]
		fieldRows[1].view = w.Fields[1]
		fieldRows[2].view = w.Fields[2]
	}

	// Three field rows: name | base URL | API key.
	for _, row := range fieldRows {
		fv := row.view
		prefix := "  "
		if fv.Focused {
			prefix = "▸ "
		}
		label := prefix + fv.Label
		labelLineStyle := fieldLabelStyle
		if fv.Focused {
			labelLineStyle = selectedStyle
		}
		if fv.Focused {
			lines = append(lines, renderSettingsSelectLine(labelLineStyle, label, innerWidth))
		} else {
			lines = append(lines, renderSettingsLine(labelLineStyle, label, innerWidth))
		}

		// Detail line: current value (or placeholder when empty).
		var detail string
		if fv.Value != "" {
			detail = "    " + fv.Value
		} else if fv.Placeholder != "" {
			detail = "    " + fv.Placeholder
		} else {
			detail = "    (empty)"
		}
		detailStyle := dimStyle
		if fv.Focused {
			detailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
		}
		lines = append(lines, detailStyle.Width(innerWidth).Render(settingsTruncate(detail, innerWidth)))

		// Hint line for read-only fields.
		if fv.Hint != "" {
			lines = append(lines, dimStyle.Italic(true).Width(innerWidth).Render("    "+settingsTruncate(fv.Hint, innerWidth)))
		}
	}

	// The text input lives at the bottom of the wizard so the user
	// always sees the cursor for the field they are editing.
	lines = append(lines, divider)
	lines = append(lines, dimStyle.Width(innerWidth).Render("  Edit current field:"))
	lines = append(lines, "  "+w.InputView)
	lines = append(lines, divider)
	lines = append(lines, dimStyle.Italic(true).Width(innerWidth).Render("  ↑/↓ select field · Enter save field · Esc save & close"))
	lines = append(lines, dimStyle.Width(innerWidth).Render("Section: Providers · F1 Sessions · F2 Workspace · F3 Settings"))

	content := strings.Join(lines, "\n")
	return s.ViewportFocusedStyle.Width(width).Height(height).Render(content)
}

type tabBarEntry struct {
	name string
	key  string
	kind TabKind
}

func tabBarEntries() []tabBarEntry {
	return []tabBarEntry{
		{"Sessions", "F1", TabKindSessions},
		{"Chat", "F2", TabKindChat},
		{"Settings", "F3", TabKindSettings},
	}
}

func tabBarPlainLabel(name, key string) string {
	return " " + name + " (" + key + ") "
}

type tabBarHitRegion struct {
	kind   TabKind
	startX int
	endX   int
}

// tabBarHitRegions mirrors renderTabBar's mid-row layout for mouse clicks.
func tabBarHitRegions() []tabBarHitRegion {
	var regions []tabBarHitRegion
	visPos := 1
	for i, d := range tabBarEntries() {
		lw := len(tabBarPlainLabel(d.name, d.key))
		if i > 0 {
			visPos++
		}
		regions = append(regions, tabBarHitRegion{
			kind:   d.kind,
			startX: visPos,
			endX:   visPos + lw + 1,
		})
		visPos += lw + 2
	}
	return regions
}

// tabKindAtX returns which tab label was clicked.
func tabKindAtX(x int) (TabKind, bool) {
	for _, r := range tabBarHitRegions() {
		if x >= r.startX && x <= r.endX {
			return r.kind, true
		}
	}
	return 0, false
}

// renderTabBar renders the tab bar: Sessions (F1) | Chat (F2) | Settings (F3).
// alertBlink is true when some session needs user attention (shown on Chat tab label).
func renderTabBar(activeTab TabKind, width int, s Styles, viewportFocused bool, alertBlink bool, hoverTab int) string {
	defs := tabBarEntries()

	// Use a consistent outline color for the tab "frames" (╭ ─ │ ╯ etc.)
	// across all tabs. The active tab is distinguished by its label style
	// (TabActiveStyle), not by changing the border/sep color.
	var sepStyle = lipgloss.NewStyle().Foreground(s.ColorWhite)

	var top, mid, bot strings.Builder
	top.WriteString(" ")
	mid.WriteString(" ")
	bot.WriteString(sepStyle.Render("╭"))
	visPos := 1

	for i, d := range defs {
		full := tabBarPlainLabel(d.name, d.key)
		lw := len(full)
		topLine := "╭" + strings.Repeat("─", lw) + "╮"
		var botLine string
		if d.kind == activeTab {
			botLine = "╯" + strings.Repeat(" ", lw) + "╰"
		} else {
			botLine = "┴" + strings.Repeat("─", lw) + "┴"
		}

		var nameStyle lipgloss.Style
		isHover := hoverTab >= 0 && int(d.kind) == hoverTab && d.kind != activeTab
		switch {
		case d.kind == activeTab:
			nameStyle = s.TabActiveStyle
		case isHover:
			nameStyle = mouseHoverStyle().Foreground(s.ColorWhite)
		case alertBlink && d.kind == TabKindSessions:
			nameStyle = s.TabAlertStyle
		default:
			nameStyle = s.TabInactiveStyle
		}
		label := " " + nameStyle.Render(d.name+" ("+d.key+")") + " "

		if i > 0 {
			top.WriteString(" ")
			mid.WriteString(" ")
			bot.WriteString(sepStyle.Render("─"))
			visPos++
		}
		top.WriteString(sepStyle.Render(topLine))
		mid.WriteString(sepStyle.Render("│") + label + sepStyle.Render("│"))
		bot.WriteString(sepStyle.Render(botLine))
		visPos += lw + 2
	}

	rem := width - visPos
	if rem < 0 {
		rem = 0
	}
	top.WriteString(strings.Repeat(" ", rem))
	mid.WriteString(strings.Repeat(" ", rem))
	if rem > 0 {
		bot.WriteString(sepStyle.Render(strings.Repeat("─", rem-1) + "╮"))
	} else {
		bot.WriteString(sepStyle.Render("╮"))
	}

	return top.String() + "\n" + mid.String() + "\n" + bot.String()
}
