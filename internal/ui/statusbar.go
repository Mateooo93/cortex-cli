package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// StatusBarInfo is the live data the slim footer needs to render
// the context / model / elapsed-time readouts. Built by the model
// from session state + cortexconfig. When fields are zero, the
// corresponding segment is omitted (no "0 tokens" clutter).
type StatusBarInfo struct {
	ModelName   string
	ProviderTag string        // short tag like "codex" or "anthropic"
	InputTokens int64
	CacheRead   int64
	ContextMax  int64
	Elapsed     time.Duration
	QueuedMsgs  int
	AutoCompact bool
	// WorkflowName + WorkflowElapsed render a "● workflow
	// <name> (2:13)" segment in the centre of the footer
	// when a workflow is running. The user sees the live
	// status without switching tabs.
	WorkflowName    string
	WorkflowStatus  string
	WorkflowElapsed time.Duration
	// GoalActive is true when a /goal is running.
	GoalActive    bool
	GoalTurns     int
	GoalCondition string
	// EffortLevel shows the current effort setting.
	EffortLevel string
}

// renderStatusBar renders the slim single-line status bar.
//
// The previous design was two lines: line 1 a wall of
// F1/F2/F3/Tab/Enter/Ctrl+T keybind badges, line 2 a keybind
// hint. That overflowed the available width on terminal < 120
// cols and looked cluttered. The new design is a single slim
// footer line: connection status · active model · context
// usage · elapsed time · F1 F2 F3 tab bar (the only keybinds
// that still need a persistent reminder).
//
// The keybind hint "Tab queue / Enter send / Esc cancel" moved
// to the right-side info panel (Ctrl+B toggle) where the user
// can read it without it competing with the rest of the UI.
func renderStatusBar(
	width int,
	connected bool,
	reconnecting bool,
	msg StatusMessage,
	s Styles,
	info StatusBarInfo,
) string {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	dimLabel := lipgloss.NewStyle().Foreground(s.ColorDimGray)

	// ── Connection status (left) ───────────────────────────────────────────
	var connStatus string
	if connected {
		connStatus = statusConnectedStyle.Render("● connected")
	} else if reconnecting {
		connStatus = statusReconnectingStyle.Render("● reconnecting")
	} else {
		connStatus = statusDisconnectedStyle.Render("● disconnected")
	}

	// ── Center: model + context + elapsed ───────────────────────────────
	centerParts := []string{}
	if info.ModelName != "" {
		modelTag := info.ModelName
		if info.ProviderTag != "" {
			modelTag = modelTag + " · " + info.ProviderTag
		}
		centerParts = append(centerParts, labelStyle.Render(modelTag))
	}
	// Context usage: "ctx 12k/200k (6%)" or "ctx 12k" if no
	// window known. Use the chars/4 fallback in buildStatusBarInfo.
	used := info.InputTokens + info.CacheRead
	if used > 0 || info.ContextMax > 0 {
		ctxSeg := "ctx "
		if info.ContextMax > 0 {
			pct := float64(used) / float64(info.ContextMax) * 100
			if pct > 100 {
				pct = 100
			}
			pctLabel := fmt.Sprintf("%.0f%%", pct)
			ctxSeg += fmt.Sprintf("%s/%s (%s)",
				formatTokenCount(used),
				formatTokenCount(info.ContextMax),
				pctLabel,
			)
		} else {
			ctxSeg += formatTokenCount(used)
		}
		// If auto-compact is on and we're close to the
		// threshold, colour the segment warning.
		ctxStyle := dimLabel
		if info.AutoCompact && info.ContextMax > 0 {
			pct := float64(used) / float64(info.ContextMax) * 100
			if pct >= 80 {
				ctxStyle = lipgloss.NewStyle().Foreground(colorWarning)
			}
		}
		centerParts = append(centerParts, ctxStyle.Render(ctxSeg))
	}
	if info.Elapsed > 0 {
		centerParts = append(centerParts, dimLabel.Render("⏱  "+formatDuration(info.Elapsed)))
	}
	if info.QueuedMsgs > 0 {
		centerParts = append(centerParts, lipgloss.NewStyle().Foreground(colorWarning).Render(fmt.Sprintf("%d queued", info.QueuedMsgs)))
	}
	if info.WorkflowName != "" {
		// Workflow is running — show a prominent
		// "● workflow <name> (2:13)" segment so the
		// user knows the orchestrator is busy even
		// when they're in the chat tab.
		wfSeg := "● workflow " + info.WorkflowName
		if info.WorkflowElapsed > 0 {
			wfSeg += " (" + formatDurationShort(info.WorkflowElapsed) + ")"
		}
		wfStyle := lipgloss.NewStyle().Foreground(colorSecondary).Bold(true)
		centerParts = append(centerParts, wfStyle.Render(wfSeg))
	}
	if info.GoalActive {
		goalSeg := fmt.Sprintf("◎ goal (%d turns)", info.GoalTurns)
		goalStyle := lipgloss.NewStyle().Foreground(colorSecondary)
		centerParts = append(centerParts, goalStyle.Render(goalSeg))
	}
	if info.EffortLevel != "" && info.EffortLevel != "high" {
		effSeg := "⚡ " + info.EffortLevel
		effStyle := lipgloss.NewStyle().Foreground(colorWarning)
		if info.EffortLevel == "ultracode" {
			effStyle = lipgloss.NewStyle().Foreground(colorSecondary).Bold(true)
			effSeg = "⚡ ultracode"
		}
		centerParts = append(centerParts, effStyle.Render(effSeg))
	}

	// ── Right: nothing (F1-F4 moved to the top tab bar) ──────────
	// The user asked for the F-key tabs at the top of the
	// screen (in the tab bar with the section names). The
	// slim footer no longer duplicates them — it stays
	// focused on connection, model, context, and elapsed.
	leftSeg := connStatus
	rightSeg := ""
	centerSeg := strings.Join(centerParts, dimLabel.Render("  "))

	leftLen := lipgloss.Width(leftSeg)
	rightLen := lipgloss.Width(rightSeg)
	centerLen := lipgloss.Width(centerSeg)
	fixed := leftLen + rightLen + 2
	remaining := width - fixed
	if remaining < 0 {
		remaining = 0
	}
	if centerLen > remaining {
		// The center segment is too long for the available
		// width — truncate from the right. The user can
		// open the right panel (Ctrl+B) for the full
		// breakdown.
		centerSeg = lipgloss.NewStyle().MaxWidth(remaining).Render(centerSeg)
		centerLen = remaining
	}
	leftPad := (remaining - centerLen) / 2
	if leftPad < 1 {
		leftPad = 1
	}
	rightPad := remaining - centerLen - leftPad
	if rightPad < 0 {
		rightPad = 0
	}

	line := leftSeg +
		strings.Repeat(" ", leftPad) +
		centerSeg +
		strings.Repeat(" ", rightPad) +
		rightSeg

	// If a transient status message is active, the
	// status bar collapses to JUST the message line
	// (1 row, not 2). The old behaviour rendered
	// message + slim footer in two lines, which
	// overlapped the bottom row of the chat viewport
	// because the layout only reserves 1 row for the
	// status bar. The user reported: "when i scroll
	// up the bottom of the chat starts disappearing
	// and at some point half of the conversation
	// section is invisible" — the bottom row was
	// being COVERED by the transient message line.
	// The fix: drop the slim footer while a message
	// is active and only show the message. The
	// connection / model readouts are still visible
	// in the right panel (Ctrl+B) for the duration
	// of the message.
	if msg.Text != "" {
		var msgStyle lipgloss.Style
		var prefix string
		switch msg.Kind {
		case StatusMsgWarning:
			msgStyle = lipgloss.NewStyle().Foreground(colorWarning).Italic(true)
			prefix = " ⚠ "
		case StatusMsgError:
			msgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
			prefix = " ✖ "
		default: // StatusMsgInfo
			// Brighter color + bold so the user
			// actually sees the update progress. The
			// user reported "there should also be an
			// animation when i update" — the old
			// dim+italic style made the message
			// almost invisible.
			msgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
			prefix = " ℹ "
		}
		// If a spinner is attached, render a braille
		// spinner in BRIGHT CYAN next to the message.
		// The status bar is now ALWAYS 1 row tall,
		// even with a spinner — no more overlap with
		// the chat viewport.
		if msg.Spinner >= 0 {
			frames := []string{"\u280b", "\u2819", "\u2838", "\u2834", "\u2826", "\u2827", "\u2807", "\u280f"}
			spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
			spinner := spinnerStyle.Render(" "+frames[msg.Spinner%len(frames)]+" ")
			content := lipgloss.NewStyle().Width(width).Render(spinner + msgStyle.Render(msg.Text))
			return content
		}
		content := lipgloss.NewStyle().Width(width).Render(msgStyle.Render(prefix + msg.Text))
		return content
	}
	_ = labelStyle // silence unused in some builds
	return s.StatusBarStyle.Width(width).Render(line)
}

// renderKeybindHint returns the always-visible keybind hint shown on
// the status bar's second line. It explains how to send, queue, and
// interrupt the agent in plain language so the user does not have to
// dig through the docs to discover the Tab-vs-Enter distinction.
//
// The hint is left-aligned (it sits in the bottom-left of the screen
// as the user requested) and uses the dim color so it does not
// compete with the connection status badge on line 1.
func renderKeybindHint(width int, s Styles) string {
	badgeStyle := lipgloss.NewStyle().Background(colorSecondary).Foreground(lipgloss.Color("0")).Bold(true)
	dimLabel := lipgloss.NewStyle().Foreground(s.ColorDimGray)
	enterBadge := badgeStyle.Render(" Enter ")
	tabBadge := badgeStyle.Render(" Tab ")
	escBadge := badgeStyle.Render(" Esc ")

	parts := []string{
		enterBadge + dimLabel.Render(" send (interrupts after current edit)  "),
		tabBadge + dimLabel.Render(" queue (run after this turn)  "),
		escBadge + dimLabel.Render(" cancel now"),
	}
	hint := strings.Join(parts, "")
	hintVisual := lipgloss.Width(hint)
	// Pad to full width so the line stays right-anchored to the
	// edge of the screen (looks tidier than a left-anchored short
	// string with the rest of the line blank).
	if hintVisual < width {
		hint = hint + strings.Repeat(" ", width-hintVisual)
	}
	return hint
}

func formatTokenCount(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%dk", n/1000)
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
