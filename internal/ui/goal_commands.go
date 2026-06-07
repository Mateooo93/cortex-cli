package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/goal"
)

// handleGoalCommand processes the /goal slash command.
//
//	/goal                  → show current goal status
//	/goal <condition>      → set a new goal
//	/goal clear            → clear the active goal
//	/goal stop|off|reset   → aliases for clear
func (m *Model) handleGoalCommand(sess *SessionState, arg string) []tea.Cmd {
	if sess == nil {
		return nil
	}

	// Lazy-init goal manager
	if sess.goalManager == nil {
		sess.goalManager = goal.NewManager(m.cortexCfg)
	}

	arg = strings.TrimSpace(arg)
	lower := strings.ToLower(arg)

	// /goal (no args) → show status
	if arg == "" {
		return m.showGoalStatus(sess)
	}

	// /goal clear|stop|off|reset|none|cancel → clear
	clearAliases := []string{"clear", "stop", "off", "reset", "none", "cancel"}
	for _, a := range clearAliases {
		if lower == a {
			sess.goalManager.Clear()
			sess.chatMessages = append(sess.chatMessages,
				renderSystemSuccessMessage("Goal cleared. The agent will stop after the current turn."))
			return nil
		}
	}

	// /goal <condition> → set new goal
	_ = sess.goalManager.Set(arg)
	sess.chatMessages = append(sess.chatMessages, renderUserMessage("/goal "+arg, 0))
	sess.chatMessages = append(sess.chatMessages,
		renderSystemSuccessMessage(fmt.Sprintf(
			"◎ Goal set. The agent will keep working until this condition is met:\n\n%s\n\n"+
				"A fast evaluator model checks progress after each turn. Run /goal to see status, /goal clear to stop.",
			arg,
		)))

	// If the main agent is not currently streaming, kick off a turn
	// with the goal as the directive
	if sess.client != nil && sess.agentState == StateWaitingForInput {
		sess.agentState = StateStreaming
		return []tea.Cmd{sess.thinkingAnim.Start(), func() tea.Msg {
			sess.client.SendInput("/goal "+arg, nil)
			return nil
		}}
	}
	return nil
}

// showGoalStatus renders the current goal state as a chat message.
func (m *Model) showGoalStatus(sess *SessionState) []tea.Cmd {
	if sess.goalManager == nil {
		sess.chatMessages = append(sess.chatMessages,
			renderSystemMessage("No goal has been set in this session.\n\nUsage: /goal <measurable condition>\nExample: /goal all tests in test/auth pass and npm test exits 0", m.styles))
		return nil
	}

	state := sess.goalManager.State()
	if state == nil {
		sess.chatMessages = append(sess.chatMessages,
			renderSystemMessage("No goal has been set in this session.", m.styles))
		return nil
	}

	switch state.Status {
	case goal.StatusActive:
		msg := fmt.Sprintf(
			"◎ Goal active\n\nCondition: %s\nTurns evaluated: %d\nLast evaluator verdict: %s",
			state.Condition, state.Turns, state.LastReason,
		)
		if state.LastReason == "" {
			msg = fmt.Sprintf(
				"◎ Goal active\n\nCondition: %s\nTurns evaluated: %d\nWaiting for first evaluation...",
				state.Condition, state.Turns,
			)
		}
		sess.chatMessages = append(sess.chatMessages, renderSystemSuccessMessage(msg))
	case goal.StatusAchieved:
		msg := fmt.Sprintf(
			"✓ Goal achieved!\n\nCondition: %s\nTurns taken: %d\nFinal verdict: %s",
			state.Condition, state.Turns, state.LastReason,
		)
		sess.chatMessages = append(sess.chatMessages, renderSystemSuccessMessage(msg))
	case goal.StatusCleared:
		msg := fmt.Sprintf(
			"Goal was cleared.\n\nCondition: %s\nTurns evaluated: %d",
			state.Condition, state.Turns,
		)
		sess.chatMessages = append(sess.chatMessages, renderSystemMessage(msg, m.styles))
	case goal.StatusFailed:
		msg := fmt.Sprintf(
			"✗ Goal failed.\n\nCondition: %s\nTurns evaluated: %d\nLast verdict: %s",
			state.Condition, state.Turns, state.LastReason,
		)
		sess.chatMessages = append(sess.chatMessages, renderErrorMessage(fmt.Errorf("%s", msg)))
	default:
		sess.chatMessages = append(sess.chatMessages,
			renderSystemMessage(fmt.Sprintf("Goal status: %s", state.Status), m.styles))
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
		"ultracode": "Ultracode — xhigh reasoning + automatic workflow orchestration. The agent will spawn parallel workflows for substantive tasks. Significantly higher token usage. Session-scoped.",
	}[level]

	sess.chatMessages = append(sess.chatMessages,
		renderSystemSuccessMessage(fmt.Sprintf(
			"Effort level set to: %s\n\n%s\n\nThis setting lasts for the current session.",
			level, description,
		)))
	return nil
}
