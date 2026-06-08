package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderTrimDialog renders the trim confirmation as a centered overlay box.
// selected: 0 = Yes, 1 = No.
func renderTrimDialog(width, height int, s Styles, selected int) string {
	dialogWidth := 50
	if dialogWidth > width-4 {
		dialogWidth = width - 4
	}
	innerWidth := dialogWidth - 4 // account for border + padding

	title := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).
		Width(innerWidth).Align(lipgloss.Center).
		Render("✂  Trim conversation?")

	sep := s.CommandPaletteSepStyle.Width(innerWidth).Render(strings.Repeat("─", innerWidth))

	msg := lipgloss.NewStyle().Foreground(s.ColorDimGray).
		Width(innerWidth).Align(lipgloss.Center).
		Render("All messages below this point will be permanently deleted.")

	yesStyle := lipgloss.NewStyle().Bold(true).Foreground(s.ColorDimGray)
	noStyle := lipgloss.NewStyle().Bold(true).Foreground(s.ColorDimGray)
	if selected == 0 {
		yesStyle = yesStyle.Foreground(colorSecondary)
	} else {
		noStyle = noStyle.Foreground(colorSecondary)
	}

	yesBtn := yesStyle.Render("Yes")
	noBtn := noStyle.Render("No")
	buttons := lipgloss.NewStyle().Width(innerWidth).Align(lipgloss.Center).
		Render(yesBtn + "    " + noBtn)

	content := title + "\n" + sep + "\n" + msg + "\n\n" + buttons

	return s.CommandPaletteStyle.Width(dialogWidth).Render(content)
}

// renderSessionCloseDialog renders the session-close confirmation as a centered overlay box.
// selected: 0 = Yes, 1 = No.
func renderSessionCloseDialog(width, height int, s Styles, selected int, sessionID string) string {
	dialogWidth := 52
	if dialogWidth > width-4 {
		dialogWidth = width - 4
	}
	innerWidth := dialogWidth - 4 // account for border + padding

	title := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).
		Width(innerWidth).Align(lipgloss.Center).
		Render("Close session?")

	sep := s.CommandPaletteSepStyle.Width(innerWidth).Render(strings.Repeat("─", innerWidth))

	body := "The session will be terminated."
	if sessionID != "" {
		body = body + "\n" + lipgloss.NewStyle().Foreground(s.ColorDimGray).Render(sessionID)
	}
	msg := lipgloss.NewStyle().Foreground(s.ColorDimGray).
		Width(innerWidth).Align(lipgloss.Center).
		Render(body)

	yesStyle := lipgloss.NewStyle().Bold(true).Foreground(s.ColorDimGray)
	noStyle := lipgloss.NewStyle().Bold(true).Foreground(s.ColorDimGray)
	if selected == 0 {
		yesStyle = yesStyle.Foreground(colorSecondary)
	} else {
		noStyle = noStyle.Foreground(colorSecondary)
	}

	yesBtn := yesStyle.Render("Yes")
	noBtn := noStyle.Render("No")
	buttons := lipgloss.NewStyle().Width(innerWidth).Align(lipgloss.Center).
		Render(yesBtn + "    " + noBtn)

	content := title + "\n" + sep + "\n" + msg + "\n\n" + buttons

	return s.CommandPaletteStyle.Width(dialogWidth).Render(content)
}

// quitDialogStyle matches the Settings tab viewport outline (white rounded
// border) while keeping a full box suitable for a centered overlay.
func quitDialogStyle(s Styles) lipgloss.Style {
	return s.ViewportFocusedStyle.BorderTop(true).Padding(1, 1)
}

// renderQuitDialog renders the quit confirmation as a centered overlay box.
// width/height are the terminal dimensions. selected: 0 = Yes, 1 = No.
func renderQuitDialog(width, height int, s Styles, selected int) string {
	dialogWidth := 44
	if dialogWidth > width-4 {
		dialogWidth = width - 4
	}
	innerWidth := dialogWidth - 4 // account for border + padding

	title := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).
		Width(innerWidth).Align(lipgloss.Center).
		Render("Quit cortex-cli?")

	sep := s.CommandPaletteSepStyle.Width(innerWidth).Render(strings.Repeat("─", innerWidth))

	msg := lipgloss.NewStyle().Foreground(s.ColorDimGray).
		Width(innerWidth).Align(lipgloss.Center).
		Render("Any running agent will be cancelled.")

	yesStyle := lipgloss.NewStyle().Bold(true).Foreground(s.ColorDimGray)
	noStyle := lipgloss.NewStyle().Bold(true).Foreground(s.ColorDimGray)
	if selected == 0 {
		yesStyle = yesStyle.Foreground(colorSecondary)
	} else {
		noStyle = noStyle.Foreground(colorSecondary)
	}

	yesBtn := yesStyle.Render("Yes")
	noBtn := noStyle.Render("No")
	buttons := lipgloss.NewStyle().Width(innerWidth).Align(lipgloss.Center).
		Render(yesBtn + "    " + noBtn)

	content := title + "\n" + sep + "\n" + msg + "\n\n" + buttons

	return quitDialogStyle(s).Width(dialogWidth).Render(content)
}
