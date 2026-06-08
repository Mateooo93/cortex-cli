package ui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// WorkflowPicker is a modal overlay for starting a multi-agent workflow.
// The user enters a task prompt and optionally picks a preset (code,
// research, test, review, docs).
type WorkflowPicker struct {
	visible  bool
	prompt   textinput.Model
	presets  []workflowPresetView
	selected int
	focus    workflowPickerFocus
}

type workflowPickerFocus int

const (
	workflowFocusPrompt workflowPickerFocus = iota
	workflowFocusPresets
)

type workflowPresetView struct {
	Name        string
	Description string
	Strategy    string
	MaxAgents   int
}

// workflowStartResult is returned when the user confirms the picker.
type workflowStartResult struct {
	Prompt string
	Preset workflowPresetView
}

// builtinPresetViews mirrors workflow.BuiltinPresets without importing
// the workflow package (avoids import cycle since workflow imports ui).
var builtinPresetViews = []workflowPresetView{
	{Name: "code", Description: "Plan, implement, review, and test a coding task end-to-end.", Strategy: "development", MaxAgents: 5},
	{Name: "research", Description: "Gather documentation and reference material, then summarise findings.", Strategy: "research", MaxAgents: 3},
	{Name: "test", Description: "Write and run tests for an existing code change.", Strategy: "testing", MaxAgents: 4},
	{Name: "review", Description: "Review a diff or plan, surface issues, and suggest fixes.", Strategy: "optimization", MaxAgents: 4},
	{Name: "docs", Description: "Write or improve project documentation (README, API docs, comments).", Strategy: "research", MaxAgents: 3},
}

// NewWorkflowPicker creates a workflow picker with a focused prompt field.
func NewWorkflowPicker() WorkflowPicker {
	ti := textinput.New()
	ti.Placeholder = "Describe what the workflow should accomplish..."
	ti.Prompt = "> "
	return WorkflowPicker{prompt: ti}
}

// Open shows the workflow picker. initialPrompt pre-fills the task field
// (e.g. from "/workflow build a CLI todo app").
func (wp *WorkflowPicker) Open(initialPrompt string) {
	wp.visible = true
	wp.presets = builtinPresetViews
	wp.focus = workflowFocusPrompt
	wp.prompt.SetValue(strings.TrimSpace(initialPrompt))
	wp.prompt.Focus()
	wp.syncPresetFromPrompt()
}

// Close hides the picker.
func (wp *WorkflowPicker) Close() {
	wp.visible = false
	wp.prompt.Blur()
}

// IsVisible returns whether the picker is showing.
func (wp *WorkflowPicker) IsVisible() bool { return wp.visible }

// Prompt returns the current task prompt text.
func (wp *WorkflowPicker) Prompt() string {
	return strings.TrimSpace(wp.prompt.Value())
}

// MoveUp moves preset selection up.
func (wp *WorkflowPicker) MoveUp() {
	if wp.selected > 0 {
		wp.selected--
	}
}

// MoveDown moves preset selection down.
func (wp *WorkflowPicker) MoveDown() {
	if wp.selected < len(wp.presets)-1 {
		wp.selected++
	}
}

// Selected returns the currently highlighted preset, or nil.
func (wp *WorkflowPicker) Selected() *workflowPresetView {
	if wp.selected < 0 || wp.selected >= len(wp.presets) {
		return nil
	}
	p := wp.presets[wp.selected]
	return &p
}

func (wp *WorkflowPicker) focusPrompt() {
	wp.focus = workflowFocusPrompt
	wp.prompt.Focus()
}

func (wp *WorkflowPicker) focusPresets() {
	wp.focus = workflowFocusPresets
	wp.prompt.Blur()
}

func (wp *WorkflowPicker) toggleFocus() {
	if wp.focus == workflowFocusPrompt {
		wp.focusPresets()
	} else {
		wp.focusPrompt()
	}
}

func (wp *WorkflowPicker) syncPresetFromPrompt() {
	picked := pickWorkflowPreset(wp.prompt.Value())
	for i, p := range wp.presets {
		if p.Name == picked.Name {
			wp.selected = i
			return
		}
	}
}

// Update handles keys when the workflow picker is open.
// Returns a start result when the user confirms, and whether the key was consumed.
func (wp *WorkflowPicker) Update(msg tea.KeyPressMsg) (*workflowStartResult, bool) {
	if !wp.visible {
		return nil, false
	}

	switch msg.String() {
	case "esc":
		wp.Close()
		return nil, true
	case "tab", "shift+tab":
		wp.toggleFocus()
		return nil, true
	}

	if wp.focus == workflowFocusPrompt {
		switch msg.String() {
		case "down", "j":
			wp.focusPresets()
			return nil, true
		case "enter":
			return wp.confirm()
		default:
			wp.prompt, _ = wp.prompt.Update(msg)
			wp.syncPresetFromPrompt()
			return nil, true
		}
	}

	switch msg.String() {
	case "up", "k":
		wp.MoveUp()
		return nil, true
	case "down", "j":
		wp.MoveDown()
		return nil, true
	case "enter":
		return wp.confirm()
	}
	return nil, true
}

func (wp *WorkflowPicker) confirm() (*workflowStartResult, bool) {
	prompt := wp.Prompt()
	if prompt == "" {
		wp.focusPrompt()
		return nil, true
	}
	sel := wp.Selected()
	if sel == nil {
		return nil, true
	}
	wp.Close()
	return &workflowStartResult{Prompt: prompt, Preset: *sel}, true
}

// View renders the workflow picker as a centered modal.
func (wp *WorkflowPicker) View(width int, styles Styles) string {
	if !wp.visible {
		return ""
	}

	modalWidth := width - 8
	if modalWidth < 48 {
		modalWidth = 48
	}
	if modalWidth > 76 {
		modalWidth = 76
	}
	innerWidth := modalWidth - 4

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colorPrimary)
	keyStyle := lipgloss.NewStyle().Foreground(colorAccentCool)

	wp.prompt.SetWidth(innerWidth)
	promptHeader := dimStyle.Render("Task prompt")
	if wp.focus == workflowFocusPrompt {
		promptHeader = titleStyle.Render("▸ Task prompt")
	}
	promptBlock := promptHeader + "\n" + wp.prompt.View()

	sep := styles.CommandPaletteSepStyle.Width(innerWidth).Render(strings.Repeat("─", innerWidth))

	presetHeader := dimStyle.Render("Preset (optional)")
	if wp.focus == workflowFocusPresets {
		presetHeader = titleStyle.Render("▸ Preset")
	}

	maxName := 0
	for _, p := range wp.presets {
		if len(p.Name) > maxName {
			maxName = len(p.Name)
		}
	}

	var presetLines []string
	for i, p := range wp.presets {
		prefix := "  "
		if i == wp.selected {
			prefix = "▸ "
		}
		nameCol := fmt.Sprintf("%-*s", maxName+2, p.Name)
		desc := p.Description
		maxDesc := innerWidth - len(prefix) - len(nameCol) - len(fmt.Sprintf(" (%d agents)", p.MaxAgents)) - 2
		if maxDesc < 10 {
			maxDesc = 10
		}
		if len(desc) > maxDesc {
			desc = desc[:maxDesc-1] + "…"
		}
		line := fmt.Sprintf("%s%s %s (%d agents)", prefix, nameCol, desc, p.MaxAgents)
		line = lipgloss.NewStyle().Width(innerWidth).Render(line)
		if i == wp.selected && wp.focus == workflowFocusPresets {
			presetLines = append(presetLines, selectedStyle.Render(line))
		} else if i == wp.selected {
			presetLines = append(presetLines, lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Width(innerWidth).Render(line))
		} else {
			presetLines = append(presetLines, dimStyle.Render(line))
		}
	}

	footer := dimStyle.Render(
		keyStyle.Render("Tab") + " switch  " +
			keyStyle.Render("↑↓") + " navigate  " +
			keyStyle.Render("Enter") + " start  " +
			keyStyle.Render("Esc") + " cancel",
	)

	content := strings.Join([]string{
		titleStyle.Render("Start workflow"),
		"",
		promptBlock,
		sep,
		presetHeader,
		strings.Join(presetLines, "\n"),
		"",
		footer,
	}, "\n")

	return styles.CommandPaletteStyle.Width(modalWidth).Render(content)
}

// startSessionWorkflow launches a workflow on the session engine.
func startSessionWorkflow(sess *SessionState, cfg *cortexconfig.Config, prompt string, preset workflowPresetView) (string, error) {
	if sess == nil {
		return "", fmt.Errorf("no session")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("empty workflow prompt")
	}
	engine := sess.EnsureWorkflowEngine(cfg)
	id, err := engine.Start(context.Background(), preset.Name, prompt, preset.Strategy, preset.MaxAgents)
	if err == nil {
		sess.activeWorkflow = id
	}
	return id, err
}