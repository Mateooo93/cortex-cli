package ui

import (
	"fmt"
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

// renderSessionsView renders the sessions list overview.
func renderSessionsView(sessions []*SessionState, width, height int, s Styles, filter, inputView string, selectedRow int) string {
	const colSession = 10
	const colRunning = 10

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
			rows = append(rows, s.TabAlertStyle.Render(dotChar+" "+plainCols))
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

func settingsSectionTitle(title string, active bool, innerWidth int) string {
	style := lipgloss.NewStyle().Bold(true)
	if active {
		style = style.Foreground(colorPrimary)
	} else {
		style = style.Foreground(colorDim)
	}
	return style.Width(innerWidth).Render(title)
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
	ShowThinking    bool
	ReasoningEffort string
	Streaming       bool
	ShowUsage       bool
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
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	activeStyle := lipgloss.NewStyle().Foreground(colorSecondary)
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	innerWidth := width - 4
	if innerWidth < 0 {
		innerWidth = 0
	}
	if inKeyInput {
		activeSection = 1
	}
	if wizard.Provider != "" {
		activeSection = 1
	}
	if activeSection < 0 || activeSection > 2 {
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
	sectionName := []string{"Models", "Providers", "Other Settings"}[activeSection]
	divider := dimStyle.Width(innerWidth).Render(strings.Repeat("─", innerWidth))
	sectionTitle := func(idx int, label string) string {
		prefix := "  "
		if activeSection == idx {
			prefix = "▸ "
		}
		return settingsSectionTitle(prefix+label, activeSection == idx, innerWidth)
	}

	activeSummary := "No model selected"
	if activeModel != "" {
		activeSummary = "Active model: " + activeModel
	}
	providerSummary := "Provider: " + activeProviderName
	if activeProviderName == "" {
		providerSummary = "Provider: —"
	}

	// When the provider edit wizard is open it fills the whole
	// Settings viewport and replaces the Models / Other Settings
	// sections. Render it here so the rest of the normal layout
	// (section switching, provider list, etc.) is suppressed.
	if wizard.Provider != "" {
		return renderSettingsWizardView(width, height, s, dimStyle, selectedStyle, activeStyle, titleStyle, innerWidth, func(text string) string { return sectionTitle(1, text) }, divider, wizard, activeSummary, providerSummary)
	}

	lines := []string{
		titleStyle.Width(innerWidth).Render("Settings"),
		activeStyle.Width(innerWidth).Render(settingsTruncate(activeSummary, innerWidth)),
		dimStyle.Width(innerWidth).Render(settingsTruncate(providerSummary+" · Tab switches section", innerWidth)),
		divider,
	}

	if inKeyInput {
		lines = append(lines,
			sectionTitle(1, "API Keys & Base URLs"),
			dimStyle.Width(innerWidth).Render(settingsTruncate(keyInputLabel, innerWidth)),
			keyInputView,
			"",
			dimStyle.Italic(true).Width(innerWidth).Render("Enter save · Esc cancel"),
		)
		return s.ViewportFocusedStyle.Width(width).Height(height).Render(strings.Join(lines, "\n"))
	}

	// Models section: provider and model columns, always visible.
	lines = append(lines,
		sectionTitle(0, "Models"),
		dimStyle.Width(innerWidth).Render("  Pick provider, then select model"),
	)
	providerColWidth := 24
	if innerWidth < 72 {
		providerColWidth = 20
	}
	if innerWidth < 52 {
		providerColWidth = 16
	}
	modelColWidth := innerWidth - providerColWidth - 4
	if modelColWidth < 18 {
		modelColWidth = 18
	}
	modelRowsLimit := clampRows(height/7, 3, 5)
	providerStart, providerEnd := settingsWindow(providerSel, len(providers), modelRowsLimit)
	modelStart, modelEnd := settingsWindow(modelSel, len(selectedModels), modelRowsLimit)

	var providerLines []string
	providerLines = append(providerLines, headerStyle.Width(providerColWidth).Render("  Provider"))
	if providerStart > 0 {
		providerLines = append(providerLines, mutedStyle.Width(providerColWidth).Render("  ↑ more"))
	}
	for i := providerStart; i < providerEnd; i++ {
		p := providers[i]
		isCursor := activeSection == 0 && modelColumn == 0 && i == providerSel
		isActiveProv := p.Name == activeProviderName
		prefix := "  "
		if isCursor {
			prefix = "▸ "
		}
		marker := ""
		if isActiveProv {
			marker = "  active"
		}
		label := settingsTruncate(prefix+p.DisplayName+marker, providerColWidth)
		// Provider names are bold and bright white so they read as headings
		// against the dim model list.
		providerNameStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
		style := providerNameStyle
		if isCursor {
			style = selectedStyle
		} else if isActiveProv {
			style = activeStyle
		}
		providerLines = append(providerLines, style.Width(providerColWidth).Render(label))
	}
	if providerEnd < len(providers) {
		providerLines = append(providerLines, mutedStyle.Width(providerColWidth).Render(fmt.Sprintf("  ↓ %d more", len(providers)-providerEnd)))
	}

	providerDisplayForModels := "Models"
	if providerSel >= 0 && providerSel < len(providers) {
		providerDisplayForModels = providers[providerSel].DisplayName + " models"
	}
	var modelLines []string
	modelLines = append(modelLines, headerStyle.Width(modelColWidth).Render("  "+settingsTruncate(providerDisplayForModels, modelColWidth-2)))
	if len(selectedModels) == 0 {
		modelLines = append(modelLines, mutedStyle.Width(modelColWidth).Render("  No models loaded"))
		modelLines = append(modelLines, mutedStyle.Width(modelColWidth).Render("  Press r to refresh"))
	} else {
		if modelStart > 0 {
			modelLines = append(modelLines, mutedStyle.Width(modelColWidth).Render("  ↑ more"))
		}
		for i := modelStart; i < modelEnd; i++ {
			mod := selectedModels[i]
			isCursor := activeSection == 0 && modelColumn == 1 && i == modelSel
			isActive := mod.Spec == activeModel
			prefix := "  "
			if isCursor {
				prefix = "▸ "
			}
			marker := ""
			if isActive {
				marker = "  active"
			}
			label := settingsTruncate(prefix+mod.DisplayName+marker, modelColWidth)
			style := mutedStyle
			if isCursor {
				style = selectedStyle
			} else if isActive {
				style = activeStyle
			}
			modelLines = append(modelLines, style.Width(modelColWidth).Render(label))
		}
		if modelEnd < len(selectedModels) {
			modelLines = append(modelLines, mutedStyle.Width(modelColWidth).Render(fmt.Sprintf("  ↓ %d more", len(selectedModels)-modelEnd)))
		}
	}
	maxModelRows := len(providerLines)
	if len(modelLines) > maxModelRows {
		maxModelRows = len(modelLines)
	}
	for len(providerLines) < maxModelRows {
		providerLines = append(providerLines, mutedStyle.Width(providerColWidth).Render(""))
	}
	for len(modelLines) < maxModelRows {
		modelLines = append(modelLines, mutedStyle.Width(modelColWidth).Render(""))
	}
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(providerLines, "\n"), lipgloss.NewStyle().Width(4).Render(""), strings.Join(modelLines, "\n")))
	if activeSection == 0 {
		lines = append(lines, dimStyle.Italic(true).Width(innerWidth).Render("  ←/→ columns · ↑/↓ move · Enter select · r refresh models"))
	}
	lines = append(lines, divider)

	// Providers section: provider name only, no status text.
	lines = append(lines,
		sectionTitle(1, "Providers"),
	)
	apiRowsLimit := clampRows(height/6, 3, 6)
	keyStart, keyEnd := settingsWindow(keySel, len(keys), apiRowsLimit)
	// Provider names are bold and bright white so they read as headings.
	providerNameStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	if keyStart > 0 {
		lines = append(lines, mutedStyle.Width(innerWidth).Render("  ↑ more providers"))
	}
	for i := keyStart; i < keyEnd; i++ {
		pk := keys[i]
		isCursor := activeSection == 1 && inspect.Provider == "" && i == keySel
		isActiveProvider := pk.Provider == activeProviderName
		prefix := "  "
		if isCursor {
			prefix = "▸ "
		}
		row := prefix + pk.DisplayName
		rowStyle := providerNameStyle
		if isCursor {
			rowStyle = selectedStyle
		} else if isActiveProvider {
			rowStyle = activeStyle
		}
		lines = append(lines, rowStyle.Width(innerWidth).Render(settingsTruncate(row, innerWidth)))
	}
	if keyEnd < len(keys) {
		lines = append(lines, mutedStyle.Width(innerWidth).Render(fmt.Sprintf("  ↓ %d more providers", len(keys)-keyEnd)))
	}
	if activeSection == 1 && inspect.Provider == "" {
		lines = append(lines, dimStyle.Italic(true).Width(innerWidth).Render("  Enter configure · r refresh models · a add provider"))
	}
	if activeSection == 1 && inspect.Provider != "" {
		// Inline detail panel for the selected provider.
		baseURLValue := inspect.BaseURL
		if baseURLValue == "" {
			baseURLValue = "(not set — will use provider default)"
		}
		keyValue := "(not set)"
		if inspect.HasAPIKey {
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
		lines = append(lines,
			mutedStyle.Width(innerWidth).Render("  ── "+inspect.DisplayName+" ──"),
			selectedStyle.Width(innerWidth).Render(fieldLabel(0, "Name")),
			mutedStyle.Width(innerWidth).Render(settingsTruncate("    "+inspect.DisplayName, innerWidth)),
			selectedStyle.Width(innerWidth).Render(fieldLabel(1, "Base URL")),
			mutedStyle.Width(innerWidth).Render(settingsTruncate("    "+baseURLValue, innerWidth)),
			selectedStyle.Width(innerWidth).Render(fieldLabel(2, "API key")),
			mutedStyle.Width(innerWidth).Render(settingsTruncate("    "+keyValue, innerWidth)),
		)
		lines = append(lines, dimStyle.Italic(true).Width(innerWidth).Render("  ↑/↓ field · Enter edit · k edit key · b edit base URL · Del clear key · r refresh · Esc back"))
	}
	lines = append(lines, divider)

	// Other Settings section, always visible.
	lines = append(lines,
		sectionTitle(2, "Other Settings"),
	)
	thinkingStatus := "Off"
	thinkingToggle := "[ ]"
	if other.ShowThinking {
		thinkingStatus = "On"
		thinkingToggle = "[✓]"
	}
	streamingStatus := "Off"
	streamingToggle := "[ ]"
	if other.Streaming {
		streamingStatus = "On"
		streamingToggle = "[✓]"
	}
	usageStatus := "Off"
	usageToggle := "[ ]"
	if other.ShowUsage {
		usageStatus = "On"
		usageToggle = "[✓]"
	}
	otherRows := []struct {
		label string
		value string
	}{
		{label: "Theme", value: normalizedSettingsValue(other.Theme)},
		{label: "Show extended thinking", value: thinkingToggle + " " + thinkingStatus},
		{label: "Reasoning effort", value: normalizedSettingsValue(other.ReasoningEffort)},
		{label: "Streaming responses", value: streamingToggle + " " + streamingStatus},
		{label: "Show token usage", value: usageToggle + " " + usageStatus},
	}
	for i, row := range otherRows {
		prefix := "  "
		if activeSection == 2 && i == otherSel {
			prefix = "▸ "
		}
		rowText := settingsTruncate(fmt.Sprintf("%s%-26s %s", prefix, row.label, row.value), innerWidth)
		rowStyle := mutedStyle
		if activeSection == 2 && i == otherSel {
			rowStyle = selectedStyle
		}
		lines = append(lines, rowStyle.Width(innerWidth).Render(rowText))
	}
	if activeSection == 2 {
		lines = append(lines, dimStyle.Italic(true).Width(innerWidth).Render("  ↑/↓ move · Enter toggle/cycle"))
	}

	lines = append(lines, divider, dimStyle.Width(innerWidth).Render(settingsTruncate("Section: "+sectionName+" · F1 Sessions · F2 Workspace · F3 Settings", innerWidth)))
	content := strings.Join(lines, "\n")
	return s.ViewportFocusedStyle.Width(width).Height(height).Render(content)
}

// renderSettingsWizardView renders the full-section provider edit
// wizard. The wizard fills the entire Settings viewport; the regular
// Models and Other Settings sections are hidden while it is open. The
// three fields are stacked vertically with the active row highlighted
// and a single text input bound to the active field sits at the bottom
// so the user can edit without a separate pop-up.
func renderSettingsWizardView(width, height int, s Styles, dimStyle, selectedStyle, activeStyle, titleStyle lipgloss.Style, innerWidth int, sectionTitle func(string) string, divider string, w SettingsWizardView, activeSummary, providerSummary string) string {
	whiteStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	fieldLabelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))

	lines := []string{
		titleStyle.Width(innerWidth).Render("Settings"),
		activeStyle.Width(innerWidth).Render(settingsTruncate(activeSummary, innerWidth)),
		dimStyle.Width(innerWidth).Render(settingsTruncate(providerSummary+" · Tab switches section", innerWidth)),
		divider,
		// The wizard is always opened from the Providers section; render
		// it as the active section title (with the ▸ marker) without
		// depending on the enclosing sectionTitle closure.
		selectedStyle.Width(innerWidth).Render("▸ Providers"),
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
		lines = append(lines, labelLineStyle.Width(innerWidth).Render(settingsTruncate(label, innerWidth)))

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

// renderTabBar renders the two-tab bar: Sessions | Chat.
// alertBlink is true when some session needs user attention (shown on Chat tab label).
func renderTabBar(activeTab TabKind, width int, s Styles, viewportFocused bool, alertBlink bool) string {
	type tabDef struct {
		label string
		kind  TabKind
	}
	defs := []tabDef{
		{" Sessions ", TabKindSessions},
		{" Workspace ", TabKindChat},
		{" Settings ", TabKindSettings},
	}

	var sepStyle lipgloss.Style
	if viewportFocused {
		sepStyle = lipgloss.NewStyle().Foreground(s.ColorWhite)
	} else {
		sepStyle = lipgloss.NewStyle().Foreground(s.ColorBlurBorder)
	}

	var top, mid, bot strings.Builder
	top.WriteString(" ")
	mid.WriteString(" ")
	bot.WriteString(sepStyle.Render("╭"))
	visPos := 1

	for i, d := range defs {
		if i > 0 {
			top.WriteString(" ")
			mid.WriteString(" ")
			bot.WriteString(sepStyle.Render("─"))
			visPos++
		}
		lw := len(d.label)
		topLine := "╭" + strings.Repeat("─", lw) + "╮"
		var botLine string
		if d.kind == activeTab {
			botLine = "╯" + strings.Repeat(" ", lw) + "╰"
		} else {
			botLine = "┴" + strings.Repeat("─", lw) + "┴"
		}

		var textStyle lipgloss.Style
		switch {
		case d.kind == activeTab:
			textStyle = s.TabActiveStyle
		case alertBlink && d.kind == TabKindSessions:
			textStyle = s.TabAlertStyle
		default:
			textStyle = s.TabInactiveStyle
		}

		top.WriteString(sepStyle.Render(topLine))
		mid.WriteString(sepStyle.Render("│") + textStyle.Render(d.label) + sepStyle.Render("│"))
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
