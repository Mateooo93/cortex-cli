package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// renderCodexAuthOverlay renders the full-screen "waiting for
// ChatGPT sign-in" overlay. We use it whenever a codex OAuth
// flow is in flight so the user has the authorize URL handy —
// in case the browser doesn't auto-open (headless / WSL /
// SSH), the user can copy the URL into any browser manually.
//
// The overlay also has a "Switch to device-code flow" hint if
// the standard browser-callback flow keeps failing (e.g. the
// "Invalid authorize request" phone-verification gate the
// user reported).
func (m *Model) renderCodexAuthOverlay() string {
	width := m.width
	height := m.height
	s := m.styles

	boxW := 70
	if width < boxW+4 {
		boxW = width - 4
	}
	if boxW < 30 {
		boxW = 30
	}

	elapsed := time.Since(m.codexAuthStartedAt).Truncate(time.Second)
	title := s.SectionTitle.Render("Waiting for ChatGPT sign-in")
	subtitle := s.DimLabel.Render(fmt.Sprintf("Switching to %s · %s elapsed", m.codexAuthModel, formatDuration(elapsed)))

	url := m.codexAuthURL
	if url == "" {
		url = "(no URL generated)"
	}
	urlText := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#1F2937")).
		Padding(0, 1).
		Width(boxW - 6).
		Render(url)

	header := s.Accent.Render("Open this URL in any browser:")
	hint1 := s.DimLabel.Render("The browser should open automatically. If it doesn't,")
	hint2 := s.DimLabel.Render("copy the URL above into Chrome / Safari / Firefox.")
	spacer := ""
	hint3 := s.DimLabel.Render("If you see \"Invalid authorize request\" (phone-verification")
	hint4 := s.DimLabel.Render("gate), run ")
	deviceHint := s.Bold.Render("/login codex --device")
	hint5 := s.DimLabel.Render(" instead — it uses a one-time")
	hint6 := s.DimLabel.Render("code that works on any account, no callback required.")
	escHint := s.DimLabel.Italic(true).Render("Esc to dismiss this overlay (sign-in continues in background)")

	lines := []string{
		title,
		subtitle,
		"",
		header,
		urlText,
		"",
		hint1,
		hint2,
		spacer,
		hint3,
		hint4,
		deviceHint,
		hint5,
		hint6,
		"",
		escHint,
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.ColorWhite).
		Padding(1, 2).
		Width(boxW).
		Render(strings.Join(lines, "\n"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
