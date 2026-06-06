package ui

import (
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
)

const slashMenuMaxVisible = 8

// slashCommands is the fixed list of built-in slash commands shown in the menu.
var slashCommands = []Command{
	{Name: "model", Description: "Switch model (any configured provider or subscription)", Action: "open_model_picker"},
	{Name: "compact", Description: "Compress conversation history to free up context window", Action: "compact_context"},
	{Name: "update", Description: "Self-update to the latest GitHub release", Action: "self_update"},
	{Name: "login", Description: "Sign in to a subscription provider (codex, claude-sub, copilot)", Action: "open_login_picker"},
	{Name: "copy", Description: "Copy conversation to clipboard", Action: "copy_conversation"},
	{Name: "clear", Description: "Clear conversation history", Action: "slash_clear"},
	{Name: "skills", Description: "List available skills", Action: "slash_skills"},
}

// SlashMenu is a popup that lists available slash commands matching the typed /query.
type SlashMenu struct {
	visible     bool
	allCommands []Command
	filtered    []Command
	selected    int
}

// Open shows the menu with the given commands filtered by query.
func (s *SlashMenu) Open(commands []Command, query string) {
	s.visible = true
	s.allCommands = commands
	s.applyFilter(query)
	s.selected = 0
}

// Refresh updates the filter query without changing the command list.
func (s *SlashMenu) Refresh(query string) {
	s.applyFilter(query)
	if s.selected >= len(s.filtered) {
		s.selected = max(0, len(s.filtered)-1)
	}
}

// applyFilter updates filtered based on query.
func (s *SlashMenu) applyFilter(query string) {
	if query == "" {
		s.filtered = s.allCommands
		return
	}
	lower := strings.ToLower(query)
	s.filtered = nil
	for _, cmd := range s.allCommands {
		if strings.Contains(strings.ToLower(cmd.Name), lower) {
			s.filtered = append(s.filtered, cmd)
		}
	}
}

// Close hides the menu.
func (s *SlashMenu) Close() {
	s.visible = false
}

// IsVisible returns whether the menu is showing.
func (s *SlashMenu) IsVisible() bool {
	return s.visible
}

// MoveUp moves the selection toward earlier entries.
func (s *SlashMenu) MoveUp() {
	if s.selected > 0 {
		s.selected--
	}
}

// MoveDown moves the selection toward later entries.
func (s *SlashMenu) MoveDown() {
	if s.selected < len(s.filtered)-1 {
		s.selected++
	}
}

// SelectedAction returns the Action of the currently highlighted command, or "" if empty.
func (s *SlashMenu) SelectedAction() string {
	if len(s.filtered) == 0 || s.selected < 0 || s.selected >= len(s.filtered) {
		return ""
	}
	return s.filtered[s.selected].Action
}

// extractSlashQuery returns the query string (text after /) and true when the
// textarea value starts with / and the suffix contains no whitespace.
func extractSlashQuery(value string) (query string, found bool) {
	if !strings.HasPrefix(value, "/") {
		return "", false
	}
	rest := value[1:]
	for _, r := range rest {
		if unicode.IsSpace(r) {
			return "", false
		}
	}
	return rest, true
}

// View renders the slash menu popup. Returns an empty string when not visible.
func (s *SlashMenu) View(width, maxHeight int, styles Styles) string {
	if !s.visible {
		return ""
	}

	maxRows := maxHeight
	if maxRows > slashMenuMaxVisible {
		maxRows = slashMenuMaxVisible
	}

	// Build top border
	borderColor := colorPrimary
	borderCharStyle := lipgloss.NewStyle().Foreground(borderColor)
	title := " Commands "
	titleStyle := lipgloss.NewStyle().Foreground(borderColor)
	titleRendered := titleStyle.Render(title)
	titleLen := lipgloss.Width(titleRendered)
	remainingDashes := width - 3 - titleLen
	if remainingDashes < 0 {
		remainingDashes = 0
	}
	topBorder := borderCharStyle.Render("╭─") + titleRendered +
		borderCharStyle.Render(strings.Repeat("─", remainingDashes)) +
		borderCharStyle.Render("╮")

	innerWidth := width - 4 // border (2) + padding (2)
	if innerWidth < 1 {
		innerWidth = 1
	}

	if len(s.filtered) == 0 {
		emptyLine := lipgloss.NewStyle().Foreground(colorDim).Render("  (no matching commands)")
		body := styles.FileCompleterStyle.Width(width).Render(emptyLine)
		return topBorder + "\n" + body
	}

	// Compute max name length from allCommands for stable column alignment
	maxNameLen := 0
	for _, cmd := range s.allCommands {
		if n := len(cmd.Name); n > maxNameLen {
			maxNameLen = n
		}
	}

	total := len(s.filtered)
	if maxRows > total {
		maxRows = total
	}

	// Sliding window around selected
	startIdx := 0
	if s.selected >= maxRows {
		startIdx = s.selected - maxRows + 1
	}
	endIdx := startIdx + maxRows
	if endIdx > total {
		endIdx = total
		startIdx = max(0, endIdx-maxRows)
	}

	var rows []string
	for i := startIdx; i < endIdx; i++ {
		cmd := s.filtered[i]
		if i == s.selected {
			nameStr := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Width(maxNameLen).Render(cmd.Name)
			descStr := lipgloss.NewStyle().Foreground(colorPrimary).Render(cmd.Description)
			line := lipgloss.NewStyle().Width(innerWidth).Render("▸ " + nameStr + "   " + descStr)
			rows = append(rows, line)
		} else {
			nameStr := lipgloss.NewStyle().Foreground(colorAccentCool).Width(maxNameLen).Render(cmd.Name)
			descStr := lipgloss.NewStyle().Foreground(colorDim).Render(cmd.Description)
			line := lipgloss.NewStyle().Width(innerWidth).Render("  " + nameStr + "   " + descStr)
			rows = append(rows, line)
		}
	}

	content := strings.Join(rows, "\n")
	body := styles.FileCompleterStyle.Width(width).Render(content)
	return topBorder + "\n" + body
}
