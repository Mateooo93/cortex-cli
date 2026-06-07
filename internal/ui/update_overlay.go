package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// updateOverlayState captures the visual state of the
// "Updating cortex" big modal that replaces the previous
// status-bar-only progress message. The user reported:
// "the /update animation should show a big pop up with a
// cool animation! and then the cli should restart once its
// ready and the tui has been updated".
//
// The overlay is rendered as a centered rounded box that
// takes about 1/3 of the screen, with:
//   - A bold title that changes phase ("Updating cortex" →
//     "All done!" → "Restarting…" → "Already up to date" /
//     "Update failed")
//   - A 4x4 braille matrix "loading" animation that morphs
//     every tick
//   - The current updater step ("Downloading v0.2.15…")
//   - A 20-cell progress bar (▮▮▮▮▯▯▯▯…) that fills as
//     the known updater steps complete
//   - Elapsed time
//   - A "step X/N" counter
//
// When the update finishes successfully the overlay
// shows a 3-second countdown ("Restarting in 3… 2… 1…")
// and then re-execs the binary so the user comes back to
// a fresh TUI running the new version. If the update
// fails or the binary is already up to date, the overlay
// shows the result and auto-dismisses (or dismisses on
// Esc / Enter).
type updateOverlayState struct {
	active        bool
	step          string
	stepIdx       int    // 0..len(updateSteps)-1
	frame         int    // animation frame, 0..7
	startedAt     time.Time
	elapsed       time.Duration
	phase         string // "running", "done", "restarting", "up-to-date", "error"
	resultMessage string
	restartIn     int // 3..0; when 0 the overlay fires syscall.Exec
}

// updateSteps lists the steps the updater reports, in order.
// We map the live step name (set by the updater goroutine) to
// the index in this slice so the progress bar fills smoothly.
// "Checking for updates" and "Fetching release metadata" are
// treated as the same step from the user's point of view
// (they're back-to-back network calls).
var updateSteps = []string{
	"Checking",
	"Downloading",
	"Verifying",
	"Installing",
}

// mapUpdateStep returns the step index for a given step name
// from the updater. Returns -1 if the name doesn't match
// anything (e.g. during the very first tick before the
// goroutine has stored anything).
func mapUpdateStep(name string) int {
	low := strings.ToLower(name)
	switch {
	case strings.Contains(low, "check"), strings.Contains(low, "fetch"), strings.Contains(low, "metadata"):
		return 0
	case strings.Contains(low, "download"):
		return 1
	case strings.Contains(low, "verif"), strings.Contains(low, "sha"), strings.Contains(low, "hash"):
		return 2
	case strings.Contains(low, "install"), strings.Contains(low, "renam"):
		return 3
	}
	return -1
}

// spinnerFrames is the 8-frame braille spinner used inside
// the overlay. We use the same set as the status bar so the
// visual language is consistent.
var updateSpinnerFrames = []string{"⣀", "⣤", "⣶", "⣿", "⣷", "⣦", "⣀", "⠉"}

// loadMorphFrames is a 4x4 braille "loading" matrix that
// morphs each frame to give the overlay a more dynamic
// feel than a single spinner glyph. Each frame is 4
// unicode-braille cells side-by-side.
var loadMorphFrames = []string{
	"⣀⣀⣀⣀",
	"⣤⣀⣀⣀",
	"⣶⣤⣀⣀",
	"⣿⣶⣤⣀",
	"⣷⣿⣶⣤",
	"⣦⣷⣿⣶",
	"⣀⣦⣷⣿",
	"⠉⣀⣦⣷",
}

// renderUpdateOverlay renders the centered modal that
// surfaces the /update flow. Returns "" if the overlay
// is not active.
func (m *Model) renderUpdateOverlay() string {
	if !m.updateOverlay.active {
		return ""
	}
	width := m.width
	height := m.height
	s := m.styles

	boxW := 64
	if width < boxW+4 {
		boxW = width - 4
	}
	if boxW < 30 {
		boxW = 30
	}

	// --- Title --------------------------------------------------------
	var title string
	var titleStyle lipgloss.Style
	switch m.updateOverlay.phase {
	case "running":
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#3B82F6"))
		title = titleStyle.Render("⟳  Updating cortex")
	case "done":
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22C55E"))
		title = titleStyle.Render("✓  All done!")
	case "restarting":
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F59E0B"))
		title = titleStyle.Render("↻  Restarting cortex")
	case "up-to-date":
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22C55E"))
		title = titleStyle.Render("✓  Already up to date")
	case "error":
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#EF4444"))
		title = titleStyle.Render("✗  Update failed")
	default:
		titleStyle = lipgloss.NewStyle().Bold(true)
		title = titleStyle.Render("Updating cortex")
	}

	// --- Big animation ----------------------------------------------
	// During running phase: animated 4x4 braille morph.
	// During restarting: countdown with a circular feel.
	// During done/error: a static large check or X.
	var anim string
	bigAnim := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#3B82F6"))
	switch m.updateOverlay.phase {
	case "running":
		frame := m.updateOverlay.frame % len(loadMorphFrames)
		anim = bigAnim.Render(loadMorphFrames[frame])
	case "restarting":
		// Show the countdown number styled large.
		bigRestart := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F59E0B"))
		anim = bigRestart.Render(fmt.Sprintf("  %d  ", m.updateOverlay.restartIn))
	case "done":
		bigDone := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22C55E"))
		anim = bigDone.Render("  ✓  ")
	case "up-to-date":
		bigDone := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22C55E"))
		anim = bigDone.Render("  ✓  ")
	case "error":
		bigErr := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#EF4444"))
		anim = bigErr.Render("  ✗  ")
	default:
		anim = bigAnim.Render(loadMorphFrames[0])
	}

	// --- Step text + counter -----------------------------------------
	var stepText string
	stepLabel := s.DimLabel.Render("Step")
	stepValue := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(m.updateOverlay.step)
	stepLine := fmt.Sprintf("%s  %s", stepLabel, stepValue)
	_ = stepText

	// --- Progress bar (only during running) -------------------------
	progressLine := ""
	if m.updateOverlay.phase == "running" {
		barW := boxW - 8
		if barW < 12 {
			barW = 12
		}
		// 4 known steps; current step is "in progress" — fill it half.
		// We map stepIdx ∈ [-1, 0..3] to a 0..1 progress fraction.
		frac := 0.0
		if m.updateOverlay.stepIdx < 0 {
			frac = 0.0
		} else {
			frac = float64(m.updateOverlay.stepIdx) / float64(len(updateSteps))
			// 25% head start within the current step.
			frac += 1.0 / float64(len(updateSteps)) * 0.5
			if frac > 1.0 {
				frac = 1.0
			}
		}
		filled := int(float64(barW) * frac)
		empty := barW - filled
		filledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#3B82F6"))
		emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#374151"))
		bar := filledStyle.Render(strings.Repeat("▮", filled)) + emptyStyle.Render(strings.Repeat("▯", empty))
		progressLine = bar
	}

	// --- Elapsed -----------------------------------------------------
	elapsed := time.Since(m.updateOverlay.startedAt).Truncate(time.Second)
	elapsedLine := s.DimLabel.Render(fmt.Sprintf("Elapsed %s", formatDuration(elapsed)))

	// --- Footer hints -----------------------------------------------
	footer := ""
	switch m.updateOverlay.phase {
	case "running":
		footer = s.DimLabel.Italic(true).Render("Please don't close the terminal.")
	case "restarting":
		footer = s.DimLabel.Italic(true).Render(fmt.Sprintf("Restarting in %d…", m.updateOverlay.restartIn))
	case "done":
		footer = s.DimLabel.Italic(true).Render("Re-executing the new binary in a moment…")
	case "up-to-date":
		footer = s.DimLabel.Italic(true).Render("Closing automatically…")
	case "error":
		footer = s.Bold.Render("Press Esc to dismiss")
	}

	// --- Result line (for done / up-to-date / error) ----------------
	resultLine := ""
	if m.updateOverlay.resultMessage != "" {
		resultStyle := s.DimLabel
		switch m.updateOverlay.phase {
		case "done", "up-to-date":
			resultStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E"))
		case "error":
			resultStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
		}
		resultLine = resultStyle.Render(m.updateOverlay.resultMessage)
	}

	// --- Assemble ---------------------------------------------------
	lines := []string{
		title,
		"",
		anim,
		"",
		stepLine,
	}
	if progressLine != "" {
		lines = append(lines, "")
		lines = append(lines, progressLine)
	}
	lines = append(lines, "")
	lines = append(lines, elapsedLine)
	if resultLine != "" {
		lines = append(lines, "")
		lines = append(lines, resultLine)
	}
	lines = append(lines, "")
	lines = append(lines, footer)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.ColorWhite).
		Padding(1, 3).
		Width(boxW).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
