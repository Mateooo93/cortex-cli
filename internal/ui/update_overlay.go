package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Mateooo93/cortex-cli/internal/updater"
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
//   - A centered braille spinner and a single progress bar under it
//   - The current updater step ("Downloading v0.2.15…")
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
	// restartPath is the exact filesystem path to the newly installed
	// binary that the updater placed (from Result.NewPath). We prefer
	// this over re-calling os.Executable() inside the old process for
	// the re-exec, because it guarantees we start the fresh build the
	// user just downloaded. Fixes cases where "update succeeds but
	// restart still runs the old version".
	restartPath string
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
	case strings.Contains(low, "npm"):
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
		title = titleStyle.Render("Updating cortex")
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

	// --- Pixel animation --------------------------------------------
	frame := m.updateOverlay.frame
	innerW := updateOverlayInnerWidth(boxW)
	var anim string
	switch m.updateOverlay.phase {
	case "running":
		anim = renderUpdateRunningAnim(frame, m.updateOverlay.stepIdx, m.updateOverlay.startedAt, innerW)
	case "restarting":
		anim = renderUpdatePixelRestart(frame, m.updateOverlay.restartIn, innerW)
	case "done", "up-to-date":
		anim = renderUpdatePixelDone(frame, innerW)
	case "error":
		anim = renderUpdatePixelError(frame, innerW)
	default:
		anim = renderUpdateRunningAnim(frame, m.updateOverlay.stepIdx, m.updateOverlay.startedAt, innerW)
	}

	// --- Step text + counter -----------------------------------------
	var stepText string
	stepLabel := s.DimLabel.Render("Step")
	stepValue := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(m.updateOverlay.step)
	stepLine := fmt.Sprintf("%s  %s", stepLabel, stepValue)
	_ = stepText

	// --- Elapsed -----------------------------------------------------
	elapsed := time.Since(m.updateOverlay.startedAt).Truncate(time.Second)
	elapsedLine := s.DimLabel.Render(fmt.Sprintf("Elapsed %s", formatDuration(elapsed)))

	// --- Footer hints -----------------------------------------------
	footer := ""
	switch m.updateOverlay.phase {
	case "running":
		footer = s.DimLabel.Italic(true).Render(
			"Usually under a minute — cortex downloads only the native binary, faster than most Node CLIs. Don't close the terminal.",
		)
	case "restarting":
		footer = s.DimLabel.Italic(true).Render(fmt.Sprintf("Restarting in %d…", m.updateOverlay.restartIn))
	case "done":
		footer = s.Bold.Render("Press Enter to restart")
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
		"",
	}
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

// --- Self-update message types and handlers ---

type selfUpdateProgressMsg struct{}

type selfUpdateFinishedMsg struct {
	result updater.Result
}

// Legacy restart-path messages; kept for test compatibility.
type updateOverlayStartRestartMsg struct{}
type updateOverlayTickMsg struct{}

type updateOverlayExecMsg struct{}

type updateOverlayDismissMsg struct{}

func selfUpdateProgressTick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return selfUpdateProgressMsg{}
	})
}

// runSelfUpdateCmd kicks off the self-update flow and opens the overlay modal.
func (m *Model) runSelfUpdateCmd() tea.Cmd {
	m.updateProgress.Store("Checking for updates\u2026")
	m.updateOverlay = updateOverlayState{
		active:    true,
		step:      "Checking for updates\u2026",
		stepIdx:   0,
		frame:     0,
		startedAt: time.Now(),
		phase:     "running",
	}
	cmds := []tea.Cmd{
		func() tea.Msg {
			res := updater.RunWithProgress(context.Background(), func(step string) {
				m.updateProgress.Store(step)
			})
			return selfUpdateFinishedMsg{result: res}
		},
		selfUpdateProgressTick(),
	}
	return tea.Batch(cmds...)
}

func (m Model) handleSelfUpdateFinished(msg selfUpdateFinishedMsg) (Model, tea.Cmd) {
	m.updateProgress.Store("")
	m.statusMsg.Spinner = -1
	switch msg.result.Kind {
	case "up-to-date":
		m.updateOverlay.phase = "up-to-date"
		m.updateOverlay.resultMessage = fmt.Sprintf("You're already running %s.", msg.result.NewVersion)
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return updateOverlayDismissMsg{}
		})
	case "updated":
		m.updateOverlay.phase = "done"
		m.updateOverlay.step = "Complete"
		m.updateOverlay.resultMessage = msg.result.Message
		m.updateOverlay.restartPath = msg.result.NewPath
		return m, nil
	default:
		errMsg := "Update failed"
		if msg.result.Error != nil {
			errMsg = msg.result.Error.Error()
		} else if msg.result.Message != "" {
			errMsg = msg.result.Message
		}
		m.updateOverlay.phase = "error"
		m.updateOverlay.resultMessage = errMsg
		return m, nil
	}
}

func (m Model) handleSelfUpdateProgress() (Model, tea.Cmd) {
	m.updateProgressFrame++
	if step, ok := m.updateProgress.Load().(string); ok && step != "" {
		if m.updateOverlay.active {
			m.updateOverlay.step = step
			m.updateOverlay.frame++
			if idx := mapUpdateStep(step); idx >= 0 {
				m.updateOverlay.stepIdx = idx
			}
		} else {
			m.statusMsg.Text = step
			m.statusMsg.Kind = StatusMsgInfo
			m.statusMsg.Spinner = m.updateProgressFrame % 8
			m.statusMsg.gen++
		}
	} else if m.updateOverlay.active && m.updateOverlay.phase == "running" {
		m.updateOverlay.frame++
	}
	if m.updateOverlay.active && m.updateOverlay.phase == "running" {
		return m, selfUpdateProgressTick()
	}
	return m, nil
}

func (m Model) handleUpdateOverlayExec() (Model, tea.Cmd) {
	if !m.updateOverlay.active {
		return m, nil
	}
	m.updateOverlay.phase = "restarting"
	return m, m.execSelfCmd()
}

func (m Model) handleUpdateOverlayDismiss() Model {
	if m.updateOverlay.active && m.updateOverlay.phase == "up-to-date" {
		m.updateOverlay.active = false
	}
	return m
}
