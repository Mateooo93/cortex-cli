package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// renderOAuthAuthOverlay renders the full-screen "waiting for sign-in"
// overlay while an OAuth flow is in flight.
func (m *Model) renderOAuthAuthOverlay() string {
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

	elapsed := time.Since(m.oauthAuthStartedAt).Truncate(time.Second)
	providerLabel := "ChatGPT"
	accountHost := "auth.openai.com"
	deviceHint := "If you see \"Invalid authorize request\" (phone-verification gate), run /login codex --device instead."
	if m.oauthAuthProvider == "xai-sub" {
		providerLabel = "xAI Grok"
		accountHost = "accounts.x.ai"
		deviceHint = "Sign in with your SuperGrok or X Premium+ account (same flow as Grok Build)."
	}

	title := s.SectionTitle.Render("Waiting for " + providerLabel + " sign-in")
	subtitle := s.DimLabel.Render(fmt.Sprintf("Switching to %s · %s elapsed", m.oauthAuthModel, formatDuration(elapsed)))

	url := m.oauthAuthURL
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
	hint3 := s.DimLabel.Render(deviceHint)
	hint4 := s.DimLabel.Render("Remote session? Forward port 56121 (xAI) or 1455 (ChatGPT) over SSH.")
	hint5 := s.DimLabel.Render("OAuth host: " + accountHost)
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
		"",
		hint3,
		hint4,
		hint5,
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