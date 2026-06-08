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

// buildWelcomeLines returns horizontally centered welcome content (logo, version,
// subtitle, shortcuts). Vertical placement is handled by welcomeViewportLines.
func buildWelcomeLines(width int, s Styles) []string {
	if width < 1 {
		width = 1
	}

	var lines []string
	if width >= cortexBannerWidth {
		for _, line := range renderCortexBanner() {
			lines = append(lines, centerDisplayLine(line, width))
		}
	} else {
		lines = append(lines, centerDisplayLine(renderCortexBannerCompact(s), width))
	}

	version := lipgloss.NewStyle().Foreground(s.ColorDimGray).Render(Version)
	lines = append(lines, centerDisplayLine(version, width), "")

	subtitle := lipgloss.NewStyle().Foreground(s.ColorWhite).Italic(true).Render("AI coding assistant")
	lines = append(lines, centerDisplayLine(subtitle, width))
	return lines
}

// welcomeViewportLines vertically centers welcome content inside the chat viewport.
func welcomeViewportLines(width, contentHeight int, s Styles) []string {
	if contentHeight < 1 {
		contentHeight = 1
	}
	lines := buildWelcomeLines(width, s)
	if len(lines) >= contentHeight {
		return lines[:contentHeight]
	}
	top := (contentHeight - len(lines)) / 2
	out := make([]string, contentHeight)
	for i := range out {
		out[i] = ""
	}
	copy(out[top:], lines)
	return out
}

// isWelcomeScreen reports whether the session is showing the empty-state welcome view.
func (m Model) isWelcomeScreen(sess *SessionState) bool {
	if m.testMode || sess == nil {
		return false
	}
	if len(sess.chatMessages) > 0 {
		return false
	}
	if sess.thinkingRendered != "" || sess.assistantRendered != "" {
		return false
	}
	if anim := sess.thinkingAnim.View(); anim != "" {
		return false
	}
	return true
}