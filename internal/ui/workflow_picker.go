package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	tea "charm.land/bubbletea/v2"
)

// WorkflowPicker is a modal overlay for selecting a workflow preset.
// It lists the 5 built-in presets (code, research, test, review, docs)
// with descriptions and lets the user pick one with keyboard or mouse.
type WorkflowPicker struct {
	visible  bool
	presets  []workflowPresetView
	selected int
	width    int
}

type workflowPresetView struct {
	Name        string
	Description string
	Strategy    string
	MaxAgents   int
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

// Open shows the workflow picker modal.
func (wp *WorkflowPicker) Open() {
	wp.visible = true
	wp.presets = builtinPresetViews
	wp.selected = 0
}

// Close hides the picker.
func (wp *WorkflowPicker) Close() {
	wp.visible = false
}

// IsVisible returns whether the picker is showing.
func (wp *WorkflowPicker) IsVisible() bool { return wp.visible }

// MoveUp moves selection up.
func (wp *WorkflowPicker) MoveUp() {
	if wp.selected > 0 {
		wp.selected--
	}
}

// MoveDown moves selection down.
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

// View renders the workflow picker as a centered modal.
func (wp *WorkflowPicker) View(styles Styles) string {
	if !wp.visible {
		return ""
	}

	// Compact modal: just the list with a title.
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colorPrimary)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	keyStyle := lipgloss.NewStyle().Foreground(colorAccentCool)

	// Compute max widths for alignment
	maxName := 0
	for _, p := range wp.presets {
		if len(p.Name) > maxName {
			maxName = len(p.Name)
		}
	}

	var lines []string
	lines = append(lines, titleStyle.Render("╭─ Workflow Presets ───────────────────────────────────────────────╮"))
	lines = append(lines, dimStyle.Render("│ Pick a workflow preset. Enter to confirm, Esc to cancel.          │"))
	lines = append(lines, dimStyle.Render("│                                                                  │"))

	for i, p := range wp.presets {
		prefix := "  "
		if i == wp.selected {
			prefix = "▸ "
		}
		agentsInfo := fmt.Sprintf("%d agents", p.MaxAgents)
		line := fmt.Sprintf("│ %s%-*s  %-*s  %s",
			prefix,
			maxName+2, p.Name,
			50, p.Description,
			agentsInfo,
		)
		// Pad to consistent width
		lineWidth := lipgloss.Width(line)
		if lineWidth < 68 {
			line += strings.Repeat(" ", 68-lineWidth)
		}
		line += "│"

		if i == wp.selected {
			lines = append(lines, selectedStyle.Render(line))
		} else {
			lines = append(lines, dimStyle.Render(line))
		}
	}

	lines = append(lines, dimStyle.Render("│                                                                  │"))
	lines = append(lines, dimStyle.Render("│ "+keyStyle.Render("↑↓")+" navigate  "+keyStyle.Render("Enter")+" start  "+keyStyle.Render("Esc")+" cancel                          │"))
	lines = append(lines, dimStyle.Render("╰──────────────────────────────────────────────────────────────────╯"))

	return strings.Join(lines, "\n")
}

// handleWorkflowPickerKey handles keys when the workflow picker is open.
// Returns the selected preset name (empty if user cancelled) and a bool
// indicating whether the key was consumed.
func (wp *WorkflowPicker) handleKey(key string) (preset string, consumed bool, cmd tea.Cmd) {
	switch key {
	case "up", "k":
		wp.MoveUp()
		return "", true, nil
	case "down", "j":
		wp.MoveDown()
		return "", true, nil
	case "enter":
		sel := wp.Selected()
		wp.Close()
		if sel != nil {
			return sel.Name, true, nil
		}
		return "", true, nil
	case "esc":
		wp.Close()
		return "", true, nil
	}
	return "", false, nil
}
