package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// activityStripSpinnerFrames is the 4-frame spinner
// used for the currently-running tool in the bottom
// activity strip. We cycle these every 1Hz tick (the
// (stale comment cleaned; workflow tick removed)
// re-renders the strip; when no tick is active, the
// strip is only re-rendered on tool events, which is
// fine because the user already knows the tool is
// running from the chat history). The 4 frames are
// braille dot rotations to keep the look consistent
// with the other spinners in the app.
var activityStripSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸"}

// activityStripMaxSummaryLen is the maximum length
// we render for a tool's Summary field in the strip.
// The user said "sub-agents should appear at the
// bottom like claude code" — Claude Code's strip
// shows the tool name + a SHORT description (often
// truncated). We follow the same pattern: anything
// longer gets an ellipsis.
const activityStripMaxSummaryLen = 40

// renderActivityStrip produces the one-line compact
// view of recent tool calls for the bottom of the
// chat tab. The user requested:
//     "sub agents run by the main ai agent should
//      appear in the bottom like claude code"
//
// Format (single line, comma-separated entries):
//   ● read_file internal/ui/model.go · 2.1s
//   ✓ edit_file foo.go (done) · 0.3s
//   ✗ run_shell npm test (failed) · 1.0s
//
// Returns an empty string when there's nothing to
// show (no recent tools) so the caller can hide the
// strip entirely and reclaim the row for the chat
// viewport.
func renderActivityStrip(s *SessionState, width int, animFrame int) string {
	if s == nil || len(s.RecentTools) == 0 {
		return ""
	}

	// Render right-to-left from the most recent
	// entry. We use a small "right-pad" strategy
	// so the most recent tool (left) is always
	// visible; older tools are dropped from the
	// left if the line gets too long.
	var entries []string
	used := 0
	for i := len(s.RecentTools) - 1; i >= 0; i-- {
		e := s.RecentTools[i]
		entry := formatActivityStripEntry(e, animFrame, i)
		entryWidth := lipgloss.Width(entry)
		// Leave room for "  " separator between
		// entries and a 1-col left margin.
		if used+entryWidth+(len(entries))*2 > width-2 {
			break
		}
		entries = append([]string{entry}, entries...)
		used += entryWidth
	}
	if len(entries) == 0 {
		return ""
	}
	return " " + strings.Join(entries, "  ")
}

// formatActivityStripEntry formats one tool call
// for the bottom strip. The status icon + name +
// summary pattern matches Claude Code's compact
// footer; the elapsed time on the right gives the
// user a sense of which tools are slow.
func formatActivityStripEntry(e RecentToolEntry, animFrame int, rowIdx int) string {
	// Status icon + tool name. The icon colour
	// matches the icon character so the user can
	// tell the status at a glance.
	var icon, iconStyle string
	switch e.Status {
	case RecentToolPending:
		// Stagger the spinner per row so multiple
		// in-flight tools shimmer in sequence,
		// not in lockstep. Math: the bottom
		// (newest) tool is row 0 in our render
		// direction, but we want row 0 (newest)
		// to cycle first; the older rows lag
		// behind. We add rowIdx to the global
		// animation frame and mod by the frame
		// count.
		frame := activityStripSpinnerFrames[(animFrame+rowIdx)%len(activityStripSpinnerFrames)]
		icon = frame
		iconStyle = "14" // bright cyan
	case RecentToolDone:
		icon = "✓"
		iconStyle = "10" // green
	case RecentToolFailed:
		icon = "✗"
		iconStyle = "9" // red
	}
	name := e.Name
	if name == "" {
		name = "tool"
	}
	summary := truncateForStrip(e.Summary, activityStripMaxSummaryLen)
	// Elapsed time / status suffix. For done
	// tools we render "(done)" instead of the
	// running timer so the user can tell the
	// tool finished; for failed tools we render
	// "(failed)". This is more legible than a
	// frozen timer ("0.3s") and matches Claude
	// Code's "✓ done" / "✗ failed" pattern.
	var suffix string
	switch e.Status {
	case RecentToolDone:
		suffix = " (done)"
	case RecentToolFailed:
		suffix = " (failed)"
	default:
		elapsed := time.Since(e.StartedAt).Truncate(100 * time.Millisecond)
		suffix = " " + formatActivityStripElapsed(elapsed)
	}
	iconStyled := lipgloss.NewStyle().Foreground(lipgloss.Color(iconStyle)).Bold(true).Render(icon)
	nameStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Render(name)
	summaryStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(summary)
	suffixStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(suffix)
	return iconStyled + " " + nameStyled + " " + summaryStyled + suffixStyled
}

// formatActivityStripElapsed formats a tool's
// running elapsed time for the strip. We keep
// this short ("2.1s", "1m02s") so the entry
// doesn't dominate the line.
func formatActivityStripElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	minutes := int(d / time.Minute)
	seconds := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%dm%02ds", minutes, seconds)
}

// truncateForStrip shortens a string to max
// characters, appending "…" if truncated. Used
// for the Summary field of each tool entry in
// the bottom activity strip.
func truncateForStrip(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return s[:max-1] + "…"
}

// orchestrationTools is the set of LLM-side
// orchestration tool names that should NOT appear
// in the bottom activity strip. These tools are
// internal coordination primitives that render as
// inline panels in the chat (a question panel, a
// todo list, a workflow picker); they don't
// represent work the user would want to see in
// the activity strip. The user-reported complaint
// was that ask_user_question rows were cluttering
// the strip with noise like:
//
//   ✓ ask_user_question header="Scope" (done)
//   ✓ ask_user_question header="Design" (done)
//   ✗ ask_user_question header="Routing" (failed)
//
// ...and the user asked us to remove them.
var orchestrationTools = map[string]bool{
	"ask_user_question": true,

	"todo_write":        true,
}

// isOrchestrationTool returns true if the given
// tool name is an LLM-orchestration primitive
// that should be hidden from the bottom activity
// strip (it has its own inline panel in the chat).
func isOrchestrationTool(name string) bool {
	return orchestrationTools[name]
}
