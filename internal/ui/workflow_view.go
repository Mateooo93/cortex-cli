package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/workflow"
)

// renderWorkflowsView renders the Workflows tab content.
//
// The tab is intentionally minimal — the user asked for a
// low-ceremony view that just shows what's running, with
// agents indented underneath each workflow. When nothing is
// running, we show a small text explaining what workflows
// are and how to start one (via /workflow <prompt>).
//
// Layout (one workflow "code" with 3 steps running):
//
//	Workflows
//
//	code   running · 2/4 steps · 0:42
//	  ●  orchestrator        planning
//	  ●  developer           writing the auth middleware
//	  ◐  reviewer            waiting on developer
//
//	Start one with /workflow <prompt>.
func renderWorkflowsView(width, height int, s Styles, engine *workflow.Engine, cursor int, animationFrame int) string {
	var out strings.Builder

	title := s.SectionTitle.Render("Workflows")
	out.WriteString(title)
	out.WriteString("\n\n")

	if engine == nil {
		return out.String()
	}

	// engine.Snapshots() returns a snapshot list. We render
	// up to (height-4) lines.
	snapshots := engine.Snapshots()
	maxLines := height - 8
	if maxLines < 4 {
		maxLines = 4
	}
	if maxLines > len(snapshots) {
		maxLines = len(snapshots)
	}

	if len(snapshots) == 0 {
		// Empty state: explain what this tab is for and
		// how to start a workflow. The user asked for
		// a low-ceremony view with no preset picker
		// clutter — just one line telling them how to
		// run a workflow.
		out.WriteString(s.DimLabel.Render("A workflow runs multiple agents in sequence to tackle a"))
		out.WriteString("\n")
		out.WriteString(s.DimLabel.Render("bigger task end-to-end (plan → implement → test)."))
		out.WriteString("\n\n")
		out.WriteString(s.DimLabel.Render("Start one with "))
		out.WriteString(s.Bold.Render("/workflow <prompt>"))
		out.WriteString("\n")
		out.WriteString(s.DimLabel.Render("Example: "))
		out.WriteString(s.Bold.Render("/workflow build a CLI todo app in Go"))
		return out.String()
	}

	// Active workflows first. We render the bold "workflow"
	// line followed by an indented list of its steps.
	for i, snap := range snapshots {
		if i >= maxLines {
			break
		}
		// Workflow name + status line.
		nameStyle := s.Bold
		marker := "  "
		if i == cursor {
			nameStyle = s.Accent
			marker = "▸ "
		}
		elapsed := time.Since(snap.StartedAt).Truncate(time.Second)
		status := snap.Status
		if status == "" {
			status = "running"
		}
		header := fmt.Sprintf("%s%s  %s · %d/%d steps · %s",
			marker, snap.Name, status, snap.DoneSteps, snap.TotalSteps, formatWorkflowElapsed(elapsed))
		out.WriteString(nameStyle.Render(header))
		out.WriteString("\n")

		// Indented agents running under this workflow. The
		// orchestrator is the first row (it's the planner /
		// coordinator for the workflow). The user reported:
		// 'the workflow agents should all run at the same
		// time and its not really clear whats happening we
		// need a time spent for every agent, what its
		// currently doing and if its actually working'. The
		// per-step row below shows: status marker, role
		// name, the live "what the agent is doing" message,
		// and a per-step elapsed timer + a braille spinner
		// for in-flight steps. Spinners cycle every tick
		// (driven by the workflowTickMsg handler) so the
		// view animates even when nothing else is changing.
		for si, step := range snap.Steps {
			stepMarker := "○ "
			stepStyle := s.DimLabel
			switch step.Status {
			case workflow.StepInProgress:
				// The spinner frame is the absolute
				// animation index from the 1Hz tick,
				// not relative to the step. This
				// makes the rows shimmer in
				// sequence rather than all flipping
				// at once.
				spinnerFrame := workflowSpinnerFrames[(si+animationFrame)%len(workflowSpinnerFrames)]
				stepMarker = spinnerFrame + " "
				stepStyle = s.Accent
			case workflow.StepDone:
				stepMarker = "✓ "
			case workflow.StepFailed:
				stepMarker = "✗ "
				stepStyle = s.Error
			case workflow.StepCancelled:
				stepMarker = "· "
			}
			// Per-step elapsed timer. The user wanted
			// 'a time spent for every agent'. Show
			// "M:SS" for active / failed steps
			// (live time), "M:SS" + "(done)" for
			// done, "—" for steps that never started.
			var stepElapsed string
			if !step.StartedAt.IsZero() {
				if !step.EndedAt.IsZero() {
					stepElapsed = formatWorkflowElapsed(step.EndedAt.Sub(step.StartedAt)) + " (done)"
				} else {
					stepElapsed = formatWorkflowElapsed(time.Since(step.StartedAt))
				}
			} else {
				stepElapsed = "—"
			}
			row := fmt.Sprintf("    %s%-12s  %-50s  %s",
				stepMarker, step.Role, truncate(formatWorkflowStepLine(step), 50), stepElapsed)
			out.WriteString(stepStyle.Render(row))
			out.WriteString("\n")
		}
	}
	return out.String()
}

// workflowSpinnerFrames is the 8-frame braille spinner
// used for in-flight workflow steps. We cycle these
// every workflowTickMsg (1Hz) so the user can see at
// a glance which agents are still working.
var workflowSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧"}

// formatWorkflowElapsed formats an elapsed duration as M:SS.
func formatWorkflowElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	minutes := int(d / time.Minute)
	seconds := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// formatWorkflowStepLine is the "what's this agent doing" sub-line
// for one step. The live currentMsg wins; otherwise we show
// the description.
func formatWorkflowStepLine(step workflow.Step) string {
	if step.CurrentMsg != "" {
		return truncate(step.CurrentMsg, 80)
	}
	return step.Description
}

// truncate is a soft right-truncate that keeps the text
// within max characters. Used so step sub-lines don't blow
// past the panel width.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}
