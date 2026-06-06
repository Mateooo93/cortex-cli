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
func renderWorkflowsView(width, height int, s Styles, engine *workflow.Engine, cursor int) string {
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
		// coordinator for the workflow).
		for _, step := range snap.Steps {
			stepMarker := "○ "
			switch step.Status {
			case workflow.StepInProgress:
				stepMarker = "● "
			case workflow.StepDone:
				stepMarker = "✓ "
			case workflow.StepFailed:
				stepMarker = "✗ "
			case workflow.StepCancelled:
				stepMarker = "· "
			}
			row := fmt.Sprintf("    %s%s  %s", stepMarker, step.Role, formatWorkflowStepLine(step))
			out.WriteString(s.DimLabel.Render(row))
			out.WriteString("\n")
		}
	}
	return out.String()
}

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
