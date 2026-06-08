package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
)

// rightPanelMode is the display mode of the right panel.
type rightPanelMode int

const (
	rpModeModel    rightPanelMode = iota // model selection list
	rpModeKeys                            // stored API key manager
	rpModeKeyInput                        // inline key entry form
	rpModeCodexSignIn                     // ChatGPT OAuth sign-in prompt
	rpModeTodos                           // pending todo list
	rpModeInfo                            // session info: model, ctx %, elapsed, keybinds
)

// RightPanelAction is the action returned by HandleKey.
type RightPanelAction int

const (
	rpActionNone          RightPanelAction = iota
	rpActionClose                           // close the panel
	rpActionModelSelected                   // payload = model API name
	rpActionKeyDeleted                      // payload = provider name
	rpActionKeyStored                       // payload = "provider:key"
	rpActionNeedKey                         // payload = "provider:pendingModel"
	rpActionCodexSignIn                     // payload = "pendingModel"
	rpActionCodexSignOut                    // payload = "" (just the provider)
)

// RightPanel is a full-height sidebar on the right side of the screen that
// contains either a model-selection list, an API key manager, or a key-input form.
type RightPanel struct {
	visible bool
	mode    rightPanelMode
	height  int

	// Model selection state
	modelSel int

	// Key manager state
	keySel int
	keys   []config.ProviderKey

	// Key input state
	keyInputProvider string
	keyInputPending  string // model name waiting for the key
	keyInput         textinput.Model

	// OAuth sign-in state (codex, xai-sub, …)
	oauthSignInProvider string // provider waiting for OAuth (e.g. "codex", "xai-sub")
	codexSignInPending  string // model spec to switch to after OAuth succeeds

	// providerConfigured is set by Model before HandleKey. When it
	// returns true, model selection skips auth prompts.
	providerConfigured func(provider string) bool

	// processLineIdx maps process_id → 0-based content line index
	// inside the info panel (rpModeInfo). Used for click-to-stop.
	processLineIdx map[string]int
}

// NewRightPanel returns a panel that is visible in info mode by
// default — the user asked for the panel to be ON from the first
// paint and Ctrl+B to hide it. Use Toggle() to flip visibility.
func NewRightPanel() RightPanel {
	return RightPanel{visible: true, mode: rpModeInfo}
}

// panelWidth is the fixed display width of the right panel.
const panelWidth = 42

// PanelWidth returns the fixed width of the right panel.
func (rp *RightPanel) PanelWidth() int { return panelWidth }

// IsVisible returns true when the panel is open.
func (rp *RightPanel) IsVisible() bool { return rp.visible }

// Close hides the panel.
func (rp *RightPanel) Close() { rp.visible = false }

// Toggle flips the panel between hidden and visible. Bound to
// Ctrl+B from the chat tab.
func (rp *RightPanel) Toggle() {
	if rp.visible {
		rp.Close()
		return
	}
	rp.OpenInfo(rp.height)
}

// OpenModelSelect opens the model selection list, pre-selecting the active model.
func (rp *RightPanel) OpenModelSelect(height int, activeModel string) {
	rp.visible = true
	rp.mode = rpModeModel
	rp.height = height
	// Pre-position cursor on the currently active model
	rp.modelSel = 0
	for i, m := range AvailableModels {
		if m.Spec == activeModel {
			rp.modelSel = i
			break
		}
	}
}

// OpenKeyManager opens the API key manager.
func (rp *RightPanel) OpenKeyManager(height int) {
	rp.visible = true
	rp.mode = rpModeKeys
	rp.keySel = 0
	rp.height = height
	rp.keys = config.ListStoredProviderKeys()
}

// OpenTodos opens the panel in todo-list mode.
func (rp *RightPanel) OpenTodos(height int) {
	rp.visible = true
	rp.mode = rpModeTodos
	rp.height = height
}

// OpenInfo opens the info / status panel. This is the default
// right-panel mode (toggled with Ctrl+B) and shows:
//   - the active model + provider
//   - context window usage (provider token counts with a
//     chars/4 fallback)
//   - elapsed time since session start
//   - a compact keybind legend so the user doesn't have to dig
//     through the status bar to find F1/F2/F3 etc.
//
// The info panel is read-only: every key (except esc) is a no-op
// so it doesn't steal focus from the chat input behind it.
func (rp *RightPanel) OpenInfo(height int) {
	rp.visible = true
	rp.mode = rpModeInfo
	rp.height = height
}

// OpenKeyInput opens the inline key-entry form for the given provider.
// pendingModel is the model the user wants to switch to once the key is saved.
func (rp *RightPanel) OpenKeyInput(provider, pendingModel string, height int) {
	rp.visible = true
	rp.mode = rpModeKeyInput
	rp.height = height
	rp.keyInputProvider = provider
	rp.keyInputPending = pendingModel

	ti := textinput.New()
	ti.Placeholder = "Paste your " + provider + " API key..."
	ti.EchoMode = textinput.EchoPassword
	ti.Focus()
	rp.keyInput = ti
}

// OpenCodexSignIn opens the inline OAuth sign-in prompt for provider.
// Pressing enter launches the browser OAuth flow. pendingModel is the
// model spec to switch to after the token is saved (may be empty).
func (rp *RightPanel) OpenCodexSignIn(provider, pendingModel string, height int) {
	rp.visible = true
	rp.mode = rpModeCodexSignIn
	rp.height = height
	rp.oauthSignInProvider = provider
	rp.codexSignInPending = pendingModel
}

// HandleKey processes a key press and returns the resulting action and its payload.
func (rp *RightPanel) HandleKey(msg tea.KeyPressMsg) (RightPanelAction, string) {
	key := msg.String()

	// Todos mode is read-only; ignore all keys.
	if rp.mode == rpModeTodos {
		return rpActionNone, ""
	}

	// ESC always closes (except during codex sign-in, which can be
	// safely cancelled — the OAuth flow only runs on enter).
	if key == "esc" {
		return rpActionClose, ""
	}

	switch rp.mode {
	case rpModeModel:
		switch key {
		case "up", "k":
			if rp.modelSel > 0 {
				rp.modelSel--
			}
		case "down", "j":
			if rp.modelSel < len(AvailableModels)-1 {
				rp.modelSel++
			}
		case "enter":
			if rp.modelSel < len(AvailableModels) {
				m := AvailableModels[rp.modelSel]
				if rp.providerConfigured != nil && rp.providerConfigured(m.Provider) {
					return rpActionModelSelected, m.Spec
				}
				// Subscription OAuth: browser sign-in when not connected.
				if m.Provider == "codex" || m.Provider == "xai-sub" {
					rp.oauthSignInProvider = m.Provider
					return rpActionCodexSignIn, m.Spec
				}
				apiKey, _ := config.ResolveProviderKey(m.Provider, true)
				if apiKey != "" {
					return rpActionModelSelected, m.Spec
				}
				// No key stored — need to request one
				return rpActionNeedKey, m.Provider + ":" + m.Spec
			}
		}

	case rpModeKeys:
		switch key {
		case "up", "k":
			if rp.keySel > 0 {
				rp.keySel--
			}
		case "down", "j":
			if rp.keySel < len(rp.keys)-1 {
				rp.keySel++
			}
		case "enter":
			if rp.keySel < len(rp.keys) {
				provider := rp.keys[rp.keySel].Provider
				// OAuth providers don't take an API key — they
				// sign in with the user's existing subscription.
				// Open the right-panel sign-in prompt instead of
				// a "paste your key" form. The handler in
				// model.go fires the browser OAuth flow.
				if cortexconfig.ProviderAuthKind(provider) == "oauth" {
					rp.oauthSignInProvider = provider
					return rpActionCodexSignIn, provider + ":"
				}
				return rpActionNeedKey, provider + ":"
			}
		case "delete", "backspace":
			if rp.keySel < len(rp.keys) {
				return rpActionKeyDeleted, rp.keys[rp.keySel].Provider
			}
		}

	case rpModeKeyInput:
		if key == "enter" {
			val := strings.TrimSpace(rp.keyInput.Value())
			if val != "" {
				return rpActionKeyStored, rp.keyInputProvider + ":" + val
			}
			return rpActionNone, ""
		}
		// Forward key to textinput
		var cmd tea.Cmd
		rp.keyInput, cmd = rp.keyInput.Update(msg)
		_ = cmd

	case rpModeCodexSignIn:
		switch key {
		case "enter":
			// Trigger the OAuth flow. The model.go handler will run
			// codex.Login() in a goroutine and close the panel.
			return rpActionCodexSignIn, rp.codexSignInPending
		case "delete", "backspace":
			// Allow the user to sign out from the same panel.
			return rpActionCodexSignOut, ""
		}
	}

	return rpActionNone, ""
}

// RightPanelInfoView is the rendered state of the info / status
// panel (rpModeInfo). Computed by the View orchestrator so the
// right panel doesn't have to know about SessionState /
// cortexconfig.
type RightPanelInfoView struct {
	ModelName    string // e.g. "GPT-5.5 (ChatGPT)"
	ProviderName string // e.g. "ChatGPT (codex)"
	InputTokens  int64  // running total of input tokens (0 if unknown)
	OutputTokens int64  // running total of output tokens
	ContextMax   int64  // model's context window in tokens (0 if unknown)
	CacheRead    int64  // total cache-read tokens (counted towards context)
	Elapsed      time.Duration
	Connected    bool
	AutoCompact  bool // "auto-compact when context > 80%" setting
	SessionCount int  // number of sessions in the sessions tab
	QueuedMsgs   int  // number of pending user messages

	// Todos is the structured todo list emitted by the
	// AI via the todo_write tool. Shown as a separate
	// block in the right panel so the user can see what
	// the agent is working on without leaving the chat.
	Todos []protocol.TodoItem

	// Processes lists background shell commands the agent started.
	Processes []protocol.BackgroundProcessItem

	// HoverProcessID is the process row under the mouse cursor.
	HoverProcessID string
}

// View renders the right panel as a bordered, full-height string.
// focused controls whether the panel border uses the focus color.
// activeModel is the currently active model API name (used to mark the selected model).
// todos is the current todo list (used in rpModeTodos).
// info is the data for the info / status mode (rpModeInfo).
func (rp *RightPanel) View(height int, s Styles, focused bool, activeModel string, todos []protocol.TodoItem, info RightPanelInfoView) string {
	innerWidth := panelWidth - 4 // border (2) + padding (2)
	rp.processLineIdx = nil

	var lines []string

	switch rp.mode {
	case rpModeModel:
		title := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Width(innerWidth).Render("Select Model")
		sep := lipgloss.NewStyle().Foreground(colorDim).Width(innerWidth).Render(strings.Repeat("─", innerWidth))
		lines = append(lines, title, sep)
		for i, m := range AvailableModels {
			label := m.DisplayName
			if m.Provider == "openai" {
				label = "[OpenAI] " + m.DisplayName
			}
			isActive := m.Spec == activeModel
			isCursor := i == rp.modelSel
			switch {
			case isCursor && isActive:
				// Cursor is on the currently active model
				line := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Width(innerWidth).Render("▸ " + label + " ✓")
				lines = append(lines, line)
			case isCursor:
				line := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Width(innerWidth).Render("▸ " + label)
				lines = append(lines, line)
			case isActive:
				// Active model without cursor focus
				line := lipgloss.NewStyle().Foreground(colorSecondary).Width(innerWidth).Render("  " + label + " ✓")
				lines = append(lines, line)
			default:
				line := lipgloss.NewStyle().Foreground(colorDim).Width(innerWidth).Render("  " + label)
				lines = append(lines, line)
			}
		}
		hint := lipgloss.NewStyle().Foreground(colorDim).Italic(true).Width(innerWidth).Render("↑/↓ navigate  Enter select  Esc close")
		lines = append(lines, "", hint)

	case rpModeKeys:
		title := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Width(innerWidth).Render("API Keys")
		sep := lipgloss.NewStyle().Foreground(colorDim).Width(innerWidth).Render(strings.Repeat("─", innerWidth))
		lines = append(lines, title, sep)
		for i, pk := range rp.keys {
			var statusStr string
			if pk.Prefix != "" {
				statusStr = pk.Prefix + "..."
			} else {
				statusStr = "(not stored)"
			}
			label := pk.Provider + ": " + statusStr
			if i == rp.keySel {
				line := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Width(innerWidth).Render("▸ " + label)
				lines = append(lines, line)
			} else {
				line := lipgloss.NewStyle().Foreground(colorDim).Width(innerWidth).Render("  " + label)
				lines = append(lines, line)
			}
		}
		hint := lipgloss.NewStyle().Foreground(colorDim).Italic(true).Width(innerWidth).Render("↑/↓ navigate  Enter add/update  Del delete  Esc close")
		lines = append(lines, "", hint)

	case rpModeKeyInput:
		title := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Width(innerWidth).Render("Enter API Key")
		sub := lipgloss.NewStyle().Foreground(colorDim).Width(innerWidth).Render("Provider: " + rp.keyInputProvider)
		sep := lipgloss.NewStyle().Foreground(colorDim).Width(innerWidth).Render(strings.Repeat("─", innerWidth))
		rp.keyInput.SetWidth(innerWidth)
		inputView := rp.keyInput.View()
		hint := lipgloss.NewStyle().Foreground(colorDim).Italic(true).Width(innerWidth).Render("Enter confirm  Esc cancel")
		lines = append(lines, title, sub, sep, inputView, "", hint)

	case rpModeCodexSignIn:
		provider := rp.oauthSignInProvider
		if provider == "" {
			provider = "codex"
		}
		titleText, bodyText := oauthSignInCopy(provider)
		title := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Width(innerWidth).Render(titleText)
		sep := lipgloss.NewStyle().Foreground(colorDim).Width(innerWidth).Render(strings.Repeat("─", innerWidth))
		body := lipgloss.NewStyle().Foreground(s.ColorWhite).Width(innerWidth).Render(bodyText)
		warn := lipgloss.NewStyle().Foreground(colorSecondary).Width(innerWidth).Render(
			"Requires xdg-open / open / wslview (Linux, macOS, WSL).")
		del := lipgloss.NewStyle().Foreground(colorDim).Width(innerWidth).Render(
			"Press Del to sign out.")
		hint := lipgloss.NewStyle().Foreground(colorDim).Italic(true).Width(innerWidth).Render("Enter sign in  Del sign out  Esc cancel")
		lines = append(lines, title, sep, body, "", warn, "", del, "", hint)

	case rpModeTodos:
		title := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Width(innerWidth).Render("Todos")
		sep := lipgloss.NewStyle().Foreground(colorDim).Width(innerWidth).Render(strings.Repeat("─", innerWidth))
		lines = append(lines, title, sep)
		for _, t := range todos {
			lines = append(lines, renderTodoOrStepLine(t.Content, string(t.Status), innerWidth))
		}

	case rpModeInfo:
		// OpenCode-style info panel. Shows a compact
		// dashboard of: active model, context window
		// usage, elapsed time, and a quick keybind
		// legend pinned to the bottom. Read-only.
		infoLines, procIdx := rp.renderInfoView(innerWidth, info, s)
		keyLines := renderInfoKeybindLines(innerWidth)
		rp.processLineIdx = procIdx
		innerHeight := height - 2
		if innerHeight < 1 {
			innerHeight = 1
		}
		lines = appendFooterPinned(infoLines, keyLines, innerHeight, 4)
	}

	// Pad to fill height (subtract 2 for border top+bottom).
	// Each element in lines may contain embedded newlines from word-wrapping, so
	// we count actual terminal lines rather than slice elements.
	if rp.mode != rpModeInfo {
		innerHeight := height - 2
		if innerHeight < 1 {
			innerHeight = 1
		}
		lines = padLinesToHeight(lines, innerHeight)
	}

	content := strings.Join(lines, "\n")
	box := rightPanelBorderStyle(s).Width(panelWidth).Height(height).Render(content)
	return box
}

// rightPanelBorderStyle uses a neutral grey outline (not theme primary).
// The top border is required so rounded corners render correctly where the
// panel meets the tab bar (BorderTop(false) left a broken gap at the top).
func rightPanelBorderStyle(s Styles) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.ColorBlurBorder).
		Padding(0, 1)
}

// renderInfoView draws the OpenCode-style info / status panel:
//   - Active model + provider (top)
//   - Context window usage bar with percentage
//   - Session stats: elapsed time, queued messages, sessions count
//   - A compact keybind legend at the bottom
//
// The view is read-only — every keypress is ignored — so the chat
// input behind the panel keeps focus and the user can keep typing
// while glancing at the stats. The bar in the context section
// uses a 20-character scale: each "▮" = 5% of the context window.
// ProcessIDAtContentLine returns the running process on the given
// 0-based content line inside the info panel, if any.
func (rp *RightPanel) ProcessIDAtContentLine(contentLine int) (string, bool) {
	for id, line := range rp.processLineIdx {
		if line == contentLine {
			return id, true
		}
	}
	return "", false
}

func (rp *RightPanel) renderInfoView(innerWidth int, info RightPanelInfoView, s Styles) ([]string, map[string]int) {
	processLineIdx := map[string]int{}
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	primaryStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	whiteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	warnStyle := lipgloss.NewStyle().Foreground(colorWarning)
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	okStyle := lipgloss.NewStyle().Foreground(colorSuccess)

	lines := []string{}
	title := " Status "
	sep := strings.Repeat("─", innerWidth)
	lines = append(lines,
		primaryStyle.Width(innerWidth).Render(title),
		dimStyle.Width(innerWidth).Render(sep),
	)

	// ── Active model block ───────────────────────────────────────
	lines = append(lines, whiteStyle.Bold(true).Width(innerWidth).Render("Model"))
	if info.ModelName != "" {
		lines = append(lines, dimStyle.Width(innerWidth).Render(truncateRight(info.ModelName, innerWidth)))
	} else {
		lines = append(lines, dimStyle.Italic(true).Width(innerWidth).Render("(none)"))
	}
	if info.ProviderName != "" {
		lines = append(lines, dimStyle.Width(innerWidth).Render(truncateRight("via "+info.ProviderName, innerWidth)))
	}

	lines = append(lines, "")

	// ── Context usage block ──────────────────────────────────────
	lines = append(lines, whiteStyle.Bold(true).Width(innerWidth).Render("Context"))
	// used = input tokens + cache-read tokens (cache counts
	// toward the model's context window).
	used := info.InputTokens
	if info.CacheRead > 0 {
		// Some providers report cache reads separately; they
		// still occupy context. Add them so the bar doesn't
		// understate usage.
		used += info.CacheRead
	}
	maxCtx := info.ContextMax
	var pct float64
	if maxCtx > 0 {
		pct = float64(used) / float64(maxCtx) * 100
		if pct > 100 {
			pct = 100
		}
	} else {
		// Unknown model — show the count but no percentage.
		pct = -1
	}
	// 20-cell bar
	const barCells = 20
	filled := 0
	if pct >= 0 {
		filled = int(pct/100*float64(barCells) + 0.5)
		if filled > barCells {
			filled = barCells
		}
	}
	bar := strings.Repeat("▮", filled) + strings.Repeat("▯", barCells-filled)
	pctLabel := "?"
	if pct >= 0 {
		pctLabel = fmt.Sprintf("%.0f%%", pct)
	}
	// Colour the bar based on how close we are to the limit.
	barColorStyle := okStyle
	switch {
	case pct >= 95:
		barColorStyle = errStyle
	case pct >= 80:
		barColorStyle = warnStyle
	}
	lines = append(lines, barColorStyle.Width(innerWidth).Render(bar))
	if maxCtx > 0 {
		lines = append(lines, dimStyle.Width(innerWidth).Render(fmt.Sprintf(
			"%s / %s (%s)",
			formatTokenCountShort(used),
			formatTokenCountShort(maxCtx),
			pctLabel,
		)))
	} else {
		lines = append(lines, dimStyle.Width(innerWidth).Render(fmt.Sprintf(
			"%s tokens (window unknown)",
			formatTokenCountShort(used),
		)))
	}
	if info.AutoCompact && pct >= 80 {
		// Pre-emptively warn the user that auto-compact will
		// fire on the next turn unless they turn it off in
		// Settings → Other Settings → Auto-compact context.
		lines = append(lines, warnStyle.Italic(true).Width(innerWidth).Render("⚠ auto-compact will run on next turn"))
	}

	lines = append(lines, "")

	// ── Session stats block ──────────────────────────────────────
	lines = append(lines, whiteStyle.Bold(true).Width(innerWidth).Render("Session"))
	lines = append(lines, dimStyle.Width(innerWidth).Render(fmt.Sprintf(
		"⏱  %s", formatDurationShort(info.Elapsed),
	)))
	if info.SessionCount > 0 {
		lines = append(lines, dimStyle.Width(innerWidth).Render(fmt.Sprintf(
			"%d session%s", info.SessionCount, plural(info.SessionCount),
		)))
	}
	if info.QueuedMsgs > 0 {
		lines = append(lines, warnStyle.Width(innerWidth).Render(fmt.Sprintf(
			"%d queued", info.QueuedMsgs,
		)))
	}

	// ── Background processes (dev servers, detached shells) ──
	if len(info.Processes) > 0 {
		lines = append(lines, "")
		lines = append(lines, whiteStyle.Bold(true).Width(innerWidth).Render("Processes"))
		lines = append(lines, dimStyle.Italic(true).Width(innerWidth).Render("click running row to stop"))
		for _, p := range info.Processes {
			cmd := truncateRight(p.Command, innerWidth-8)
			if p.Running {
				elapsed := time.Since(time.Unix(p.StartedAt, 0)).Truncate(time.Second)
				line := fmt.Sprintf("● pid %d  %s  %s", p.PID, cmd, formatDurationShort(elapsed))
				processLineIdx[p.ID] = len(lines)
				rowStyle := warnStyle
				if p.ID == info.HoverProcessID {
					rowStyle = warnStyle.Strikethrough(true).Background(lipgloss.Color("#3A2A2A"))
				}
				lines = append(lines, rowStyle.Width(innerWidth).Render(truncateRight(line, innerWidth)))
			} else {
				line := fmt.Sprintf("○ %s  exited %d", cmd, p.ExitCode)
				lines = append(lines, dimStyle.Width(innerWidth).Render(truncateRight(line, innerWidth)))
			}
		}
	}

	// ── Todos (only when the AI has emitted a todo list) ──
	if len(info.Todos) > 0 {
		lines = append(lines, "")
		lines = append(lines, whiteStyle.Bold(true).Width(innerWidth).Render("Todos"))
		for _, t := range info.Todos {
			lines = append(lines, renderTodoOrStepLine(t.Content, string(t.Status), innerWidth))
		}
	}

	return lines, processLineIdx
}

func renderInfoKeybindLines(innerWidth int) []string {
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	whiteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)

	lines := []string{
		"",
		whiteStyle.Bold(true).Width(innerWidth).Render("Keys"),
	}
	for _, row := range rightPanelKeybindRows {
		lines = append(lines, renderKeybindLegendRow(keyStyle, dimStyle, row[0], row[1], innerWidth))
	}
	return lines
}

func countTermLines(ss []string) int {
	n := 0
	for _, s := range ss {
		n += strings.Count(s, "\n") + 1
	}
	return n
}

func padLinesToHeight(lines []string, height int) []string {
	for countTermLines(lines) < height {
		lines = append(lines, "")
	}
	for len(lines) > 0 && countTermLines(lines) > height {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// appendFooterPinned keeps footer lines at the bottom of height,
// inserting spacer lines above. Body lines are trimmed from the
// end first when the combined content overflows.
func appendFooterPinned(body, footer []string, height, minGap int) []string {
	if minGap < 0 {
		minGap = 0
	}
	bodyH := countTermLines(body)
	footerH := countTermLines(footer)
	gap := height - bodyH - footerH
	if gap < minGap {
		gap = minGap
	}
	if bodyH+gap+footerH > height {
		trim := bodyH + gap + footerH - height
		body = trimTermLinesFromEnd(body, trim)
		bodyH = countTermLines(body)
		gap = height - bodyH - footerH
		if gap < 0 {
			gap = 0
		}
	}
	out := append([]string{}, body...)
	for i := 0; i < gap; i++ {
		out = append(out, "")
	}
	return append(out, footer...)
}

func trimTermLinesFromEnd(lines []string, trim int) []string {
	if trim <= 0 || len(lines) == 0 {
		return lines
	}
	for trim > 0 && len(lines) > 0 {
		last := lines[len(lines)-1]
		lineCount := strings.Count(last, "\n") + 1
		if trim >= lineCount {
			trim -= lineCount
			lines = lines[:len(lines)-1]
			continue
		}
		parts := strings.Split(last, "\n")
		parts = parts[:len(parts)-trim]
		lines[len(lines)-1] = strings.Join(parts, "\n")
		trim = 0
	}
	return lines
}

// rightPanelKeybindRows lists panel-specific shortcuts. F-keys and input
// keys (Tab/Enter/Esc) live on the tab bar and under the chat input.
var rightPanelKeybindRows = [][2]string{
	{"Ctrl+T", "new session"},
	{"Ctrl+B", "toggle panel"},
	{"Ctrl+C", "quit"},
	{"/", "slash menu"},
}

func renderKeybindLegendRow(keyStyle, dimStyle lipgloss.Style, key, action string, innerWidth int) string {
	keyPart := keyStyle.Render(key)
	line := keyPart + dimStyle.Render("  "+truncateRight(action, innerWidth-lipgloss.Width(keyPart)-2))
	return lipgloss.NewStyle().Width(innerWidth).Render(line)
}

// truncateRight is like settingsTruncate but right-side cut and
// doesn't add "…" for an exact-fit string.
func truncateRight(text string, width int) string {
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

// formatTokenCountShort formats a token count as "1.2k" / "234k"
// / "1.5M" so it fits in the right panel's 38-character inner
// width.
func formatTokenCountShort(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 10000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%dk", n/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
}

// formatDurationShort formats a duration as "0:42" / "1:23:45"
// — the same format the chat turn-info line uses.
func formatDurationShort(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// plural returns "s" if n != 1 (English pluralization helper).
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// renderTodoOrStepLine renders a single labelled item with a status icon, wrapped to innerWidth.
// status values: "pending", "in_progress", "completed", "failed".
func renderTodoOrStepLine(label, status string, innerWidth int) string {
	var bullet, text string
	switch status {
	case "in_progress":
		bullet = lipgloss.NewStyle().Foreground(colorSecondary).Render("▶ ")
		text = lipgloss.NewStyle().Foreground(colorSecondary).Width(innerWidth - 2).Render(label)
	case "completed":
		bullet = lipgloss.NewStyle().Foreground(colorSuccess).Render("✓ ")
		text = lipgloss.NewStyle().Foreground(colorDim).Width(innerWidth - 2).Render(label)
	case "failed":
		bullet = lipgloss.NewStyle().Foreground(colorError).Render("✗ ")
		text = lipgloss.NewStyle().Foreground(colorError).Width(innerWidth - 2).Render(label)
	default: // pending
		bullet = lipgloss.NewStyle().Foreground(colorDim).Render("○ ")
		text = lipgloss.NewStyle().Foreground(colorDim).Width(innerWidth - 2).Render(label)
	}
	// Indent continuation lines to align under the text, not the bullet.
	textLines := strings.Split(text, "\n")
	result := bullet + textLines[0]
	for _, l := range textLines[1:] {
		result += "\n  " + l
	}
	return result
}

func oauthSignInCopy(provider string) (title, body string) {
	switch provider {
	case "xai-sub":
		return "Connect SuperGrok / X",
			"Use your SuperGrok or X Premium+ subscription — no API key from console.x.ai. " +
				"Press Enter to open accounts.x.ai in your browser. After you approve, " +
				"cortex-cli stores the token in your OS keychain (same flow as Grok Build)."
	default:
		return "Sign in with ChatGPT",
			"Use your ChatGPT subscription to power cortex-cli — no OpenAI API key. " +
				"Press Enter to open the sign-in page in your browser. After you approve, " +
				"cortex-cli stores the token in your OS keychain."
	}
}
