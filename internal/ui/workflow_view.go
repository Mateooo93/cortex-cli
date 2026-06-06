package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/Mateooo93/cortex-cli/internal/workflow"
)

// renderWorkflowsView renders the Workflows tab content.
//
// The tab has three sections:
//
//   - The header: a one-line summary of the active workflow
//     (name, status, elapsed time, "X/Y steps done").
//   - The list: every workflow the engine knows about, with
//     the active one highlighted. The user can navigate
//     up/down to inspect past workflows.
//   - The detail: for the selected workflow, a per-step
//     breakdown showing role, description, status, and
//     elapsed time. The currently-running step shows a
//     live "currentMsg" line so the user can see what the
//     agent is doing right now.
//
// Keys:
//
//   \u2191/\u2193       navigate workflow list
//   Enter         start a new workflow with the highlighted preset
//   n             start a new workflow (opens goal prompt)
//   c             cancel the selected workflow
//   s             stop the currently-running step in the selected workflow
//   Tab           switch focus between list and detail
//   Esc           close the Workflows tab
func renderWorkflowsView(
	width, height int,
	s Styles,
	engine *workflow.Engine,
	selectedIdx int,
	presets []workflow.Preset,
	cursorPreset int,
	newGoalMode bool,
	newGoalInput string,
	activeTab int, // 0 = list, 1 = detail
) string {
	dimStyle := lipgloss.NewStyle().Foreground(colorDim)
	whiteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	primaryStyle := lipgloss.NewStyle().Foreground(colorPrimary)
	activeStyle := lipgloss.NewStyle().Bold(true).Foreground(colorSecondary)
	warnStyle := lipgloss.NewStyle().Foreground(colorWarning)
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	okStyle := lipgloss.NewStyle().Foreground(colorSuccess)
	innerWidth := width - 4
	if innerWidth < 0 {
		innerWidth = 0
	}
	divider := dimStyle.Width(innerWidth).Render(strings.Repeat("\u2500", innerWidth))

	lines := []string{
		titleStyle.Width(innerWidth).Render("Workflows"),
		dimStyle.Width(innerWidth).Render("Run multi-agent workflows: plan, develop, review, and test in parallel."),
		divider,
	}

	// \u2500\u2500 New goal prompt \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500
	if newGoalMode {
		lines = append(lines,
			activeStyle.Width(innerWidth).Render("\u25b8 New workflow"),
			dimStyle.Width(innerWidth).Render("  Pick a preset:"),
		)
		for i, p := range presets {
			prefix := "  "
			if i == cursorPreset {
				prefix = "\u25b8 "
			}
			row := fmt.Sprintf("%s%-12s %s", prefix, p.Name, p.Description)
			rowStyle := whiteStyle
			if i == cursorPreset {
				rowStyle = activeStyle
			}
			lines = append(lines, rowStyle.Width(innerWidth).Render(settingsTruncate(row, innerWidth)))
		}
		lines = append(lines,
			"",
			dimStyle.Width(innerWidth).Render("  Enter start \u00b7 Esc cancel"),
		)
		content := strings.Join(lines, "\n")
		return s.ViewportFocusedStyle.Width(width).Height(height).Render(content)
	}

	// \u2500\u2500 Workflow list \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500
	flows := engine.Workflows()
	if len(flows) == 0 {
		lines = append(lines,
			dimStyle.Italic(true).Width(innerWidth).Render("  No workflows yet. Press n to start one, or mention \"workflow\" in chat and the assistant will dispatch one."),
		)
	} else {
		lines = append(lines, whiteStyle.Bold(true).Width(innerWidth).Render("Workflows"))
		for i, w := range flows {
			snap := engine.Snapshot(w.ID)
			isSel := i == selectedIdx && activeTab == 0
			prefix := "  "
			if isSel {
				prefix = "\u25b8 "
			}
			name := snap.Name
			if name == "" {
				name = "(unnamed)"
			}
			statusStr := snap.Status
			statusStyle := dimStyle
			switch snap.Status {
			case workflow.StepDone:
				statusStr = "done"
				statusStyle = okStyle
			case workflow.StepFailed:
				statusStr = "failed"
				statusStyle = errStyle
			case workflow.StepCancelled:
				statusStr = "cancelled"
				statusStyle = warnStyle
			case "running":
				statusStr = "running"
				statusStyle = activeStyle
			case "planning":
				statusStr = "planning\u2026"
				statusStyle = activeStyle
			case "synthesizing":
				statusStr = "synthesising\u2026"
				statusStyle = activeStyle
			}
			dur := ""
			if !snap.StartedAt.IsZero() {
				end := snap.EndedAt
				if end.IsZero() {
					end = time.Now()
				}
				dur = "  " + formatDurationShort(end.Sub(snap.StartedAt))
			}
			// Step progress: "3/5" steps done.
			progress := ""
			if total := len(snap.Steps); total > 0 {
				done := 0
				for _, st := range snap.Steps {
					if st.Status == workflow.StepDone {
						done++
					}
				}
				progress = fmt.Sprintf("  %d/%d steps", done, total)
			}
			row := fmt.Sprintf("%s%-22s %s%s%s", prefix, name, statusStyle.Render(statusStr), progress, dimStyle.Render(dur))
			rowStyle := whiteStyle
			if isSel {
				rowStyle = activeStyle
			}
			lines = append(lines, rowStyle.Width(innerWidth).Render(settingsTruncate(row, innerWidth)))
		}
	}

	// \u2500\u2500 Detail panel \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500
	lines = append(lines, divider)
	if len(flows) == 0 {
		// No workflow selected \u2014 show the preset list as a hint.
		lines = append(lines, whiteStyle.Bold(true).Width(innerWidth).Render("Presets"))
		for i, p := range presets {
			prefix := "  "
			if i == cursorPreset {
				prefix = "\u25b8 "
			}
			row := fmt.Sprintf("%s%-12s %s", prefix, p.Name, p.Description)
			rowStyle := dimStyle
			if i == cursorPreset {
				rowStyle = primaryStyle
			}
			lines = append(lines, rowStyle.Width(innerWidth).Render(settingsTruncate(row, innerWidth)))
		}
		lines = append(lines, "")
		lines = append(lines, dimStyle.Italic(true).Width(innerWidth).Render("  Press n to start one of these. The chat agent can also dispatch workflows automatically."))
	} else {
		idx := selectedIdx
		if idx >= len(flows) {
			idx = len(flows) - 1
		}
		if idx < 0 {
			idx = 0
		}
		wf := flows[idx]
		snap := engine.Snapshot(wf.ID)
		lines = append(lines, activeStyle.Width(innerWidth).Render("\u25b8 " + snap.Name))
		// Status header
		statusLine := "  status: "
		switch snap.Status {
		case workflow.StepDone:
			statusLine += okStyle.Render("done")
		case workflow.StepFailed:
			statusLine += errStyle.Render("failed")
		case workflow.StepCancelled:
			statusLine += warnStyle.Render("cancelled")
		case "running":
			statusLine += activeStyle.Render("running")
		case "planning":
			statusLine += activeStyle.Render("planning\u2026")
		case "synthesizing":
			statusLine += activeStyle.Render("synthesising\u2026")
		default:
			statusLine += dimStyle.Render(snap.Status)
		}
		if !snap.StartedAt.IsZero() {
			end := snap.EndedAt
			if end.IsZero() {
				end = time.Now()
			}
			statusLine += dimStyle.Render(fmt.Sprintf("  \u23f1 %s", formatDurationShort(end.Sub(snap.StartedAt))))
		}
		lines = append(lines, dimStyle.Width(innerWidth).Render(statusLine))
		// Goal
		lines = append(lines, dimStyle.Width(innerWidth).Render("  goal: "+settingsTruncate(snap.Goal, innerWidth-8)))
		// Current message (only when active)
		if snap.CurrentMsg != "" && (snap.Status == "running" || snap.Status == "synthesizing" || snap.Status == "planning") {
			lines = append(lines, activeStyle.Width(innerWidth).Render("  \u25b6 "+snap.CurrentMsg))
		}
		lines = append(lines, divider)
		// Per-step breakdown
		if len(snap.Steps) == 0 {
			lines = append(lines, dimStyle.Italic(true).Width(innerWidth).Render("  (planning\u2026)"))
		} else {
			for _, st := range snap.Steps {
				var bullet, dur string
				rowStyle := dimStyle
				switch st.Status {
				case workflow.StepDone:
					bullet = okStyle.Render("\u2713 ")
					rowStyle = whiteStyle
				case workflow.StepFailed:
					bullet = errStyle.Render("\u2717 ")
					rowStyle = errStyle
				case workflow.StepCancelled:
					bullet = warnStyle.Render("\u00d8 ")
				case workflow.StepInProgress:
					bullet = activeStyle.Render("\u25b6 ")
					rowStyle = activeStyle
				default:
					bullet = dimStyle.Render("\u25cb ")
				}
				if st.Duration > 0 {
					dur = dimStyle.Render(fmt.Sprintf("  %s", formatDurationShort(st.Duration)))
				}
				row := fmt.Sprintf("  %s %-10s %s%s", bullet, st.Role, settingsTruncate(st.Description, innerWidth-20), dur)
				lines = append(lines, rowStyle.Width(innerWidth).Render(row))
			}
		}
		// Summary
		if snap.Summary != "" && (snap.Status == workflow.StepDone || snap.Status == workflow.StepFailed || snap.Status == workflow.StepCancelled) {
			lines = append(lines, divider)
			lines = append(lines, whiteStyle.Bold(true).Width(innerWidth).Render("Summary"))
			// Word-wrap the summary to innerWidth using a
			// soft-wrap so long LLM outputs don't blow
			// up the layout.
			for _, line := range softWrap(snap.Summary, innerWidth-4) {
				lines = append(lines, dimStyle.Width(innerWidth).Render("  "+line))
			}
		}
	}
	lines = append(lines, divider)
	lines = append(lines, dimStyle.Width(innerWidth).Render("  \u2191/\u2193 navigate \u00b7 Enter open detail \u00b7 n new \u00b7 c cancel workflow \u00b7 s stop step \u00b7 F2 chat"))

	content := strings.Join(lines, "\n")
	return s.ViewportFocusedStyle.Width(width).Height(height).Render(content)
}

// softWrap breaks a string into lines no longer than `width`.
// Word-aware when possible; falls back to hard wrap at width
// runes when a single word is longer than the limit.
func softWrap(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		words := strings.Fields(para)
		var line string
		for _, w := range words {
			if line == "" {
				line = w
				continue
			}
			if len(line)+1+len(w) <= width {
				line += " " + w
			} else {
				out = append(out, line)
				line = w
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
