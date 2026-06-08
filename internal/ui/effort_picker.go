package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// EffortPicker is a modal overlay for choosing session reasoning effort.
type EffortPicker struct {
	visible  bool
	levels   []effortLevelView
	selected int
}

type effortLevelView struct {
	ID          string
	Label       string
	Description string
}

var effortLevelViews = []effortLevelView{
	{ID: "low", Label: "Low", Description: "Faster responses, less thorough. Good for simple questions."},
	{ID: "medium", Label: "Medium", Description: "Balanced speed and thoroughness."},
	{ID: "high", Label: "High", Description: "More thorough analysis and planning (default)."},
	{ID: "ultracode", Label: "Ultracode", Description: "Maximum reasoning depth for complex tasks. Higher token usage."},
}

// Open shows the effort picker with the current level pre-selected.
func (ep *EffortPicker) Open(currentLevel string) {
	ep.visible = true
	ep.levels = effortLevelViews
	ep.selected = effortLevelIndex(currentLevel)
}

func effortLevelIndex(level string) int {
	level = strings.ToLower(strings.TrimSpace(level))
	for i, l := range effortLevelViews {
		if l.ID == level {
			return i
		}
	}
	// Default to High.
	for i, l := range effortLevelViews {
		if l.ID == "high" {
			return i
		}
	}
	return 0
}

// Close hides the picker.
func (ep *EffortPicker) Close() {
	ep.visible = false
}

// IsVisible returns whether the picker is showing.
func (ep *EffortPicker) IsVisible() bool { return ep.visible }

// MoveUp moves selection up.
func (ep *EffortPicker) MoveUp() {
	if ep.selected > 0 {
		ep.selected--
	}
}

// MoveDown moves selection down.
func (ep *EffortPicker) MoveDown() {
	if ep.selected < len(ep.levels)-1 {
		ep.selected++
	}
}

// Selected returns the highlighted effort level ID, or "".
func (ep *EffortPicker) Selected() string {
	if ep.selected < 0 || ep.selected >= len(ep.levels) {
		return ""
	}
	return ep.levels[ep.selected].ID
}

// View renders the effort picker as a centered modal.
func (ep *EffortPicker) View(styles Styles) string {
	if !ep.visible {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colorPrimary)
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	keyStyle := lipgloss.NewStyle().Foreground(colorAccentCool)

	maxLabel := 0
	for _, l := range ep.levels {
		if len(l.Label) > maxLabel {
			maxLabel = len(l.Label)
		}
	}

	var lines []string
	lines = append(lines, titleStyle.Render("╭─ Reasoning Effort ───────────────────────────────────────────────╮"))
	lines = append(lines, dimStyle.Render("│ Choose how much reasoning the agent uses for this session.       │"))
	lines = append(lines, dimStyle.Render("│                                                                  │"))

	for i, l := range ep.levels {
		prefix := "  "
		if i == ep.selected {
			prefix = "▸ "
		}
		line := fmt.Sprintf("│ %s%-*s  %s",
			prefix,
			maxLabel+2, l.Label,
			l.Description,
		)
		lineWidth := lipgloss.Width(line)
		if lineWidth < 68 {
			line += strings.Repeat(" ", 68-lineWidth)
		}
		line += "│"
		if i == ep.selected {
			lines = append(lines, selectedStyle.Render(line))
		} else {
			lines = append(lines, dimStyle.Render(line))
		}
	}

	lines = append(lines, dimStyle.Render("│                                                                  │"))
	lines = append(lines, dimStyle.Render("│ "+keyStyle.Render("↑↓")+" navigate  "+keyStyle.Render("Enter")+" apply  "+keyStyle.Render("Esc")+" cancel                          │"))
	lines = append(lines, dimStyle.Render("╰──────────────────────────────────────────────────────────────────╯"))

	return strings.Join(lines, "\n")
}

// handleKey handles keys when the effort picker is open.
// Returns the selected level ID (empty if cancelled) and whether the key was consumed.
func (ep *EffortPicker) handleKey(key string) (level string, consumed bool) {
	switch key {
	case "up", "k":
		ep.MoveUp()
		return "", true
	case "down", "j":
		ep.MoveDown()
		return "", true
	case "enter":
		level = ep.Selected()
		ep.Close()
		return level, true
	case "esc":
		ep.Close()
		return "", true
	}
	return "", false
}