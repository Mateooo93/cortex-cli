package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// handleGoalCommand processes the /goal slash command.
//
// The session (internal/session) owns the real goal loop. The TUI
// simply forwards /goal commands to the session via SendInput and
// shows status messages in chat.
//
//	/goal                  → show current goal status
//	/goal <condition>      → set a new goal (session starts autonomous loop)
//	/goal clear            → clear the active goal
func (m *Model) handleGoalCommand(sess *SessionState, arg string) []tea.Cmd {
	if sess == nil {
		return nil
	}

	arg = strings.TrimSpace(arg)
	lower := strings.ToLower(arg)

	// /goal (no args) → show status. The session's GoalState()
	// mirrors what the daemon-side session is tracking.
	if arg == "" {
		if sess.client == nil {
			return nil
		}
		cond, active, turns, verdict := sess.client.GoalState()
		if !active && cond == "" {
			sess.chatMessages = append(sess.chatMessages,
				renderSystemMessage("No goal has been set in this session.\n\nUsage: /goal <measurable condition>\nExample: /goal all tests pass and npm test exits 0", m.styles))
			return nil
		}
		if active {
			msg := fmt.Sprintf("◎ Goal active\n\nCondition: %s\nTurns evaluated: %d\nLast verdict: %s",
				cond, turns, verdict)
			if verdict == "" {
				msg = fmt.Sprintf("◎ Goal active\n\nCondition: %s\nTurns evaluated: %d\nWaiting for first evaluation...",
					cond, turns)
			}
			sess.chatMessages = append(sess.chatMessages, renderSystemSuccessMessage(msg))
		} else {
			msg := fmt.Sprintf("Goal inactive. Condition: %s\nTurns: %d\nLast verdict: %s",
				cond, turns, verdict)
			sess.chatMessages = append(sess.chatMessages, renderSystemMessage(msg, m.styles))
		}
		return nil
	}

	// /goal clear|stop|off|reset|none|cancel → clear
	clearAliases := []string{"clear", "stop", "off", "reset", "none", "cancel"}
	for _, a := range clearAliases {
		if lower == a {
			if sess.client != nil {
				sess.client.SendCancel()
			}
			sess.chatMessages = append(sess.chatMessages,
				renderSystemSuccessMessage("Goal cleared. The agent will stop after the current turn."))
			return nil
		}
	}

	// /goal <condition> → set new goal. The session handles the
	// autonomous loop internally via its Send("/goal ...") path.
	sess.chatMessages = append(sess.chatMessages, renderUserMessage("/goal "+arg, 0))
	sess.chatMessages = append(sess.chatMessages,
		renderSystemSuccessMessage(fmt.Sprintf(
			"◎ Goal set. The agent will keep working autonomously until:\n\n%s\n\n"+
				"A fast evaluator checks progress after each turn. /goal to see status, /goal clear to stop.",
			arg,
		)))

	if sess.client != nil && sess.agentState == StateWaitingForInput {
		sess.agentState = StateStreaming
		return []tea.Cmd{sess.thinkingAnim.Start(), func() tea.Msg {
			sess.client.SendInput("/goal "+arg, nil)
			return nil
		}}
	}
	return nil
}

// handleEffortCommand processes the /effort slash command.
//
//	/effort             → show current effort level
//	/effort low         → set low effort
//	/effort medium      → set medium effort
//	/effort high        → set high effort
//	/effort ultracode   → set xhigh + auto-workflow (ultracode mode)
func (m *Model) handleEffortCommand(sess *SessionState, arg string) []tea.Cmd {
	if sess == nil {
		return nil
	}

	arg = strings.TrimSpace(strings.ToLower(arg))

	validLevels := map[string]string{
		"low":       "low",
		"medium":    "medium",
		"high":      "high",
		"ultracode": "ultracode",
	}

	// /effort (no args) → show current level
	if arg == "" {
		level := sess.effortLevel
		if level == "" {
			level = "high (default)"
		}
		sess.chatMessages = append(sess.chatMessages,
			renderSystemMessage(fmt.Sprintf(
				"Current effort level: %s\n\nSet with /effort <low|medium|high|ultracode>",
				level,
			), m.styles))
		return nil
	}

	level, ok := validLevels[arg]
	if !ok {
		sess.chatMessages = append(sess.chatMessages,
			renderSystemMessage(
				"Invalid effort level. Use: low, medium, high, or ultracode",
				m.styles,
			))
		return nil
	}

	sess.effortLevel = level

	description := map[string]string{
		"low":       "Low effort — faster responses, less thorough. Good for simple questions.",
		"medium":    "Medium effort — balanced speed and thoroughness.",
		"high":      "High effort — more thorough analysis and planning.",
		"ultracode": "Ultracode — xhigh reasoning + automatic workflow orchestration. Every substantive task spawns a parallel workflow. Significantly higher token usage. Session-scoped.",
	}[level]

	sess.chatMessages = append(sess.chatMessages,
		renderSystemSuccessMessage(fmt.Sprintf(
			"Effort level set to: %s\n\n%s\n\nThis setting lasts for the current session.",
			level, description,
		)))
	return nil
}
