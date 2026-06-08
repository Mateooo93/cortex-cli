package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// updateOverlayPad accounts for border (2) + Padding(1,3) horizontal (6).
const updateOverlayPad = 8

// updateBrailleFrames is the large Heroku-style braille spinner used by
// charmbracelet/bubbles (spinner.Dot) and github.com/6/braille-pattern-cli-
// loading-indicator. A single glyph cycling reads cleanly in a centered modal.
var updateBrailleFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// updateOverlayInnerWidth is the content column inside the update modal.
func updateOverlayInnerWidth(boxW int) int {
	inner := boxW - updateOverlayPad
	if inner < 20 {
		inner = 20
	}
	return inner
}

// centerUpdateAnim centers each animation line inside the modal.
func centerUpdateAnim(content string, innerW int) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		plain := stripANSI(line)
		if strings.TrimSpace(plain) == "" {
			continue
		}
		n := len([]rune(plain))
		if n >= innerW {
			continue
		}
		left := (innerW - n) / 2
		right := innerW - n - left
		lines[i] = strings.Repeat(" ", left) + line + strings.Repeat(" ", right)
	}
	return strings.Join(lines, "\n")
}

// updateAnimProgress maps updater steps + time to a 0..1 fraction for the bar.
func updateAnimProgress(stepIdx, frame int, startedAt time.Time) float64 {
	steps := float64(len(updateSteps))
	if steps <= 0 {
		steps = 4
	}
	base := 0.0
	if stepIdx >= 0 {
		base = float64(stepIdx) / steps
	}
	stepSpan := 1.0 / steps
	stepPulse := (float64(frame%28) / 28.0) * stepSpan * 0.85
	elapsed := time.Since(startedAt).Seconds()
	timePulse := math.Min(stepSpan*0.35, elapsed*0.004)
	frac := base + stepPulse + timePulse
	if stepIdx < len(updateSteps)-1 && frac > 0.92 {
		frac = 0.92
	}
	if frac > 1.0 {
		frac = 1.0
	}
	if frac < 0.06 {
		frac = 0.06
	}
	return frac
}

// renderUpdateBrailleSpinner is a single centered braille glyph (bubbles
// spinner.Dot). One character cycling — no stacked duplicate frames.
func renderUpdateBrailleSpinner(frame, innerW int) string {
	cur := updateBrailleFrames[frame%len(updateBrailleFrames)]
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#60A5FA")).Bold(true)
	return centerUpdateAnim(style.Render(cur), innerW)
}

// renderUpdateMeterBar is a single progress bar — fill grows with the
// updater step; only the leading edge pulses so it never looks frozen.
func renderUpdateMeterBar(frame, stepIdx int, startedAt time.Time, innerW int) string {
	frac := updateAnimProgress(stepIdx, frame, startedAt)
	fillEnd := int(math.Round(float64(innerW) * frac))

	filledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#3B82F6"))
	brightStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#93C5FD")).Bold(true)
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#1E293B"))

	var b strings.Builder
	for i := 0; i < innerW; i++ {
		switch {
		case fillEnd > 0 && i == fillEnd-1:
			if frame%2 == 0 {
				b.WriteString(brightStyle.Render("█"))
			} else {
				b.WriteString(filledStyle.Render("▓"))
			}
		case i < fillEnd-1:
			b.WriteString(filledStyle.Render("█"))
		default:
			b.WriteString(emptyStyle.Render("░"))
		}
	}
	return b.String()
}

// renderUpdateRunningAnim shows one spinner above the progress bar.
func renderUpdateRunningAnim(frame, stepIdx int, startedAt time.Time, innerW int) string {
	bar := renderUpdateMeterBar(frame, stepIdx, startedAt, innerW)
	return renderUpdateBrailleSpinner(frame, innerW) + "\n" + bar
}

func renderUpdatePixelDone(frame, innerW int) string {
	// Simple centered check — no lopsided pixel sprite.
	bright := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80")).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E"))
	mark := bright.Render("✓")
	if frame%6 < 3 {
		mark = dim.Render("✓")
	}
	return centerUpdateAnim(mark, innerW)
}

func renderUpdatePixelError(frame, innerW int) string {
	bright := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#7F1D1D"))
	mark := bright.Render("✗")
	if frame%4 >= 2 {
		mark = dim.Render("✗")
	}
	return centerUpdateAnim(mark, innerW)
}

func renderUpdatePixelRestart(frame, seconds, innerW int) string {
	if seconds > 0 {
		digit := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F59E0B")).
			Render(fmt.Sprintf("%d", seconds))
		return centerUpdateAnim(digit, innerW)
	}
	return renderUpdateBrailleSpinner(frame, innerW)
}