package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"image/color"
)

// Version is the cortex-cli version string rendered on the welcome screen.
// Set by the build pipeline via -ldflags -X main.Version=...
var Version = "dev"

const cortexBannerWidth = 58

// renderCortexBanner returns the CORTEX ASCII art with a vertical blue gradient.
func renderCortexBanner() []string {
	lines := []string{
		" ██████╗ ██████╗ ██████╗ ████████╗███████╗██╗  ██╗",
		"██╔════╝██╔═══██╗██╔══██╗╚══██╔══╝██╔════╝╚██╗██╔╝",
		"██║     ██║   ██║██████╔╝   ██║   █████╗   ╚███╔╝ ",
		"██║     ██║   ██║██╔══██╗   ██║   ██╔══╝   ██╔██╗ ",
		"╚██████╗╚██████╔╝██║  ██║   ██║   ███████╗██╔╝ ██╗",
		" ╚═════╝ ╚═════╝ ╚═╝  ╚═╝   ╚═╝   ╚══════╝╚═╝  ╚═╝",
	}
	ramp := []color.Color{
		lipgloss.Color("#00E5FF"),
		lipgloss.Color("#00B4FF"),
		lipgloss.Color("#0080FF"),
		lipgloss.Color("#0066FF"),
		lipgloss.Color("#0047FF"),
		lipgloss.Color("#0033CC"),
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		style := lipgloss.NewStyle().Foreground(ramp[i])
		out[i] = style.Render(line)
	}
	return out
}

func renderCortexBannerCompact(s Styles) string {
	style := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	return style.Render("CORTEX")
}

func centerDisplayLine(line string, width int) string {
	if width <= 0 {
		return line
	}
	w := lipgloss.Width(line)
	if w >= width {
		return line
	}
	pad := (width - w) / 2
	return strings.Repeat(" ", pad) + line
}

// renderWelcomeInline renders a centered welcome message for the empty chat state.
// Each output line is kept within width so resize does not re-wrap the ASCII logo.
func renderWelcomeInline(width, height int, s Styles) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	var lines []string
	if width >= cortexBannerWidth {
		lines = append(lines, renderCortexBanner()...)
	} else {
		lines = append(lines, centerDisplayLine(renderCortexBannerCompact(s), width))
	}

	version := lipgloss.NewStyle().Foreground(s.ColorDimGray).Render(Version)
	lines = append(lines, centerDisplayLine(version, width), "")

	subtitle := lipgloss.NewStyle().Foreground(s.ColorWhite).Italic(true).Render("AI coding assistant")
	lines = append(lines, centerDisplayLine(subtitle, width), "")

	shortcutStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(s.ColorWhite)
	shortcuts := []struct {
		key  string
		desc string
	}{
		{"Enter", "Send (interrupt after current edit)"},
		{"Tab", "Queue message for next turn"},
		{"Shift+Tab", "Cycle mode"},
		{"Ctrl+N", "Next session"},
		{"Ctrl+P", "Previous session"},
		{"Ctrl+R", "Search history"},
		{"Ctrl+C", "Quit"},
		{"Esc", "Cancel current operation"},
	}
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
	rowWidth := maxKeyWidth + 2 + maxDescWidth
	for _, sc := range shortcuts {
		key := shortcutStyle.Width(maxKeyWidth).AlignHorizontal(lipgloss.Right).Render(sc.key)
		desc := descStyle.Width(maxDescWidth).Render(sc.desc)
		row := lipgloss.NewStyle().Width(rowWidth).Render(key + "  " + desc)
		lines = append(lines, centerDisplayLine(row, width))
	}

	// Vertical centering without lipgloss Height(), which re-wraps content on resize.
	contentRows := 0
	for _, line := range lines {
		contentRows += visualRows(line, width)
	}
	if height > contentRows {
		topPad := (height - contentRows) / 2
		padded := make([]string, 0, topPad+len(lines))
		for i := 0; i < topPad; i++ {
			padded = append(padded, "")
		}
		padded = append(padded, lines...)
		lines = padded
	}

	return strings.Join(lines, "\n")
}