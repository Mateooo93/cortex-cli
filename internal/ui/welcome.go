package ui

import (
	"charm.land/lipgloss/v2" // Version is the vix version string rendered on the welcome screen. Set by
	"image/color"
	"strings"
)

// cmd/vix/main.go at startup from the ldflags-provided build version.
var Version = "dev" // renderVixBanner returns the CORTEX ASCII art with a vertical blue gradient.
// The logo is 6 lines tall and ~58 columns wide, matching the original VIX
// dimensions so the welcome screen layout doesn't shift.
func renderVixBanner() string {
	// Block letters for C O R T E X. 6 lines, 58 columns.
	lines := []string{
		" ██████╗ ██████╗ ██████╗ ████████╗███████╗██╗  ██╗",
		"██╔════╝██╔═══██╗██╔══██╗╚══██╔══╝██╔════╝╚██╗██╔╝",
		"██║     ██║   ██║██████╔╝   ██║   █████╗   ╚███╔╝ ",
		"██║     ██║   ██║██╔══██╗   ██║   ██╔══╝   ██╔██╗ ",
		"╚██████╗╚██████╔╝██║  ██║   ██║   ███████╗██╔╝ ██╗",
		" ╚═════╝ ╚═════╝ ╚═╝  ╚═╝   ╚═╝   ╚══════╝╚═╝  ╚═╝",
	}
	// Blue-focused gradient: bright cyan → sky blue → electric blue → royal blue → deep blue → navy
	ramp := []color.Color{
		lipgloss.Color("#00E5FF"), // bright cyan
		lipgloss.Color("#00B4FF"), // sky blue
		lipgloss.Color("#0080FF"), // electric blue
		lipgloss.Color("#0066FF"), // royal blue
		lipgloss.Color("#0047FF"), // deep blue
		lipgloss.Color("#0033CC"), // navy blue
	}
	var result strings.Builder
	for i, line := range lines {
		style := lipgloss.NewStyle().Foreground(ramp[i])
		result.WriteString(style.Render(line))
		result.WriteRune('\n')
	}
	return result.String()
} // renderWelcomeInline renders a centered welcome message for inline mode.
func renderWelcomeInline(width, height int, s Styles) string {
	// Build the welcome block (uncentered)
	var block strings.Builder
	block.WriteString(renderVixBanner())
	version := lipgloss.NewStyle().Foreground(s.ColorDimGray).Render(Version)
	block.WriteString(version + "\n\n")
	subtitle := lipgloss.NewStyle().Foreground(s.ColorWhite).Italic(true).Render("AI coding assistant")
	block.WriteString(subtitle + "\n\n")
	shortcutStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(s.ColorWhite)
	shortcuts := []struct {
		key  string
		desc string
	}{{"Enter", "Send (interrupt after current edit)"}, {"Tab", "Queue message for next turn"}, {"Shift+Tab", "Cycle mode"}, {"Ctrl+N", "Next session"}, {"Ctrl+P", "Previous session"}, {"Ctrl+R", "Search history"}, {"Ctrl+C", "Quit"}, {"Esc", "Cancel current operation"}} // Find the longest key and longest desc to build fixed-width rows
	maxKeyWidth := 0
	maxDescWidth := 0
	for _, sc := range shortcuts {
		if len(sc.key) > maxKeyWidth {
			maxKeyWidth = len(sc.key)
		}
		if len(sc.desc) > maxDescWidth {
			maxDescWidth = len(sc.desc)
		}
	}
	rowWidth := maxKeyWidth + 2 + maxDescWidth // key + gap + desc
	for _, sc := range shortcuts {
		key := shortcutStyle.Width(maxKeyWidth).AlignHorizontal(lipgloss.Right).Render(sc.key)
		desc := descStyle.Width(maxDescWidth).Render(sc.desc)
		row := lipgloss.NewStyle().Width(rowWidth).Render(key + "  " + desc)
		block.WriteString(row + "\n")
	} // Center horizontally and vertically
	centered := lipgloss.NewStyle().Width(width).Height(height).AlignHorizontal(lipgloss.Center).AlignVertical(lipgloss.Center).Render(block.String())
	return centered
}
