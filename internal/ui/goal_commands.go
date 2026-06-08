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
func slashCommandNameForAction(action string) string {
	for _, cmd := range slashCommands {
		if cmd.Action == action {
			return cmd.Name
		}
	}
	return ""
}

func (m *Model) handleGoalCommand(sess *SessionState, arg string) []tea.Cmd {
	if sess == nil {
		return nil
	}

	arg = slashCommandArgs(arg, "goal")
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

	// If the goal looks like a multi-step task, also start a workflow
	// alongside the goal loop. The workflow handles planning/execution
	// while the goal evaluator judges the final result.
	workflowStarted := false
	lowerGoal := strings.ToLower(arg)
	if isSubstantivePrompt(lowerGoal) || detectWorkflowIntent(lowerGoal) {
		preset := pickWorkflowPreset(lowerGoal)
		if _, err := startSessionWorkflow(sess, m.cortexCfg, arg, preset); err == nil {
			workflowStarted = true
		}
	}

	goalMsg := fmt.Sprintf(
		"◎ Goal set. The agent will keep working autonomously until:\n\n%s\n\n"+
			"A fast evaluator checks progress after each turn. /goal to see status, /goal clear to stop.",
		arg,
	)
	if workflowStarted {
		goalMsg += fmt.Sprintf("\n\n⚡ Workflow started alongside goal. Switch to Workflows tab (F4) to see progress.")
	}
	sess.chatMessages = append(sess.chatMessages, renderSystemSuccessMessage(goalMsg))

	if sess.client != nil {
		if sess.agentState != StateWaitingForInput {
			sess.client.SendCancelAfterEdit()
		}
		sess.agentState = StateStreaming
		sess.StartTurn()
		goalInput := "/goal"
		if arg != "" {
			goalInput = "/goal " + arg
		}
		input := goalInput
		return []tea.Cmd{sess.thinkingAnim.Start(), func() tea.Msg {
			sess.client.SendInput(input, nil)
			return nil
		}}
	}
	return nil
}

// openEffortPicker shows the effort level picker overlay.
func (m *Model) openEffortPicker(sess *SessionState) []tea.Cmd {
	current := ""
	if sess != nil {
		current = sess.effortLevel
	}
	m.effortPicker.Open(current)
	if sess != nil {
		sess.input.Blur()
	}
	return nil
}

// applyEffortLevel sets the session effort level and syncs provider reasoning.
func (m *Model) applyEffortLevel(sess *SessionState, level string) {
	if sess == nil || level == "" {
		return
	}

	sess.effortLevel = level
	switch level {
	case "low", "medium", "high":
		m.setActiveReasoningEffort(level)
	case "ultracode":
		m.setActiveReasoningEffort("xhigh")
	}

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
}
