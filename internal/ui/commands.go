package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
)

// slashCommandFromLine parses "/goal all tests pass" into name "goal" and
// args "all tests pass". Returns ok=false when the line is not a slash command.
func slashCommandFromLine(line string) (name, args string, ok bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "/") {
		return "", "", false
	}
	rest := strings.TrimSpace(line[1:])
	if rest == "" {
		return "", "", false
	}
	space := strings.IndexFunc(rest, unicodeIsSpace)
	if space < 0 {
		return strings.ToLower(rest), "", true
	}
	name = strings.ToLower(strings.TrimSpace(rest[:space]))
	args = strings.TrimSpace(rest[space:])
	return name, args, true
}

func unicodeIsSpace(r rune) bool {
	return r == ' ' || r == '\t'
}

// slashCommandArgs extracts the argument portion from a slash input line.
// For "/goal all tests pass" with command "goal" it returns "all tests pass".
// If the line does not start with the command prefix, the whole trimmed line
// is returned (so callers can pass either the full line or just the args).
func slashCommandArgs(fullLine, command string) string {
	fullLine = strings.TrimSpace(fullLine)
	if fullLine == "" {
		return ""
	}
	prefix := "/" + strings.ToLower(strings.TrimSpace(command))
	if strings.HasPrefix(strings.ToLower(fullLine), prefix) {
		return strings.TrimSpace(fullLine[len(prefix):])
	}
	return fullLine
}

func slashActionForName(name string) (string, bool) {
	for _, cmd := range slashCommands {
		if cmd.Name == name {
			return cmd.Action, true
		}
	}
	return "", false
}

// tryDispatchSlashInput routes a full slash command line (e.g. "/goal tests pass")
// through handleCommandAction. Returns handled=true when the line matched a
// built-in slash command.
func (m *Model) tryDispatchSlashInput(sess *SessionState, line string) ([]tea.Cmd, bool) {
	name, args, ok := slashCommandFromLine(line)
	if !ok {
		return nil, false
	}
	action, found := slashActionForName(name)
	if !found {
		return nil, false
	}
	return m.handleCommandAction(action, sess, args), true
}

// handleCommandAction executes the command identified by action and returns any
// resulting tea.Cmd values. It is shared by the command palette and slash menu.
// rawArg is the slash-command argument text (everything after the command name,
// e.g. for "/goal all tests pass" the arg is "all tests pass"). Pass "" for
// commands that don't take arguments.
func (m *Model) handleCommandAction(action string, sess *SessionState, rawArg ...string) []tea.Cmd {
	var arg string
	if len(rawArg) > 0 {
		arg = strings.TrimSpace(rawArg[0])
	}
	var cmds []tea.Cmd
	switch action {
	case "compact_context":
		// /compact slash command. Fire the compaction in
		// a goroutine so the TUI stays responsive. We
		// emit a 'compacting…' status up front so the
		// user sees something happen — the LLM summary
		// call takes 5-15s and without this the TUI
		// looks frozen. handleCompactMsg replaces it
		// with the final 'done compacting' / 'compacted
		// 142k → 4k tokens' result.
		//
		// In-flight guard: repeated /compact presses
		// (e.g. the user mashing Enter on the slash
		// command) used to queue multiple compactions
		// against the same session state, which could
		// corrupt the history. CodeRabbit flagged this
		// in PR #2 — we now check compactInFlight
		// before starting and surface a "compaction
		// already running" status if it's set.
		if m.compactInFlight {
			cmds = append(cmds, m.emitStatusMsg("compaction already running…", StatusMsgInfo))
			break
		}
		m.compactInFlight = true
		cmds = append(cmds, m.emitStatusMsg("compacting context…", StatusMsgInfo))
		cmds = append(cmds, m.compactCmd())
	case "open_model_picker":
		// Open the centered /model picker overlay. The picker
		// itself handles its own key events (filter, navigation,
		// selection) via m.modelPicker in the main Update loop.
		m.modelPicker.Open(m.cortexCfg)
		// Blur the chat input so the picker gets all keystrokes.
		if sess != nil {
			sess.input.Blur()
		}
	case "open_login_picker":
		// Open the /login picker overlay (subscription sign-in).
		m.loginPicker.Open()
		if sess != nil {
			sess.input.Blur()
		}
	case "self_update":
		cmds = append(cmds, m.runSelfUpdateCmd())
	case "change_model":
		if sess != nil {
			sess.rightPanel.OpenModelSelect(m.height, sess.modelName)
			m.updateChatWidth()
			sess.focus = FocusRightPanel
			sess.input.Blur()
		}
	case "manage_keys":
		if sess != nil {
			sess.rightPanel.OpenKeyManager(m.height)
			m.updateChatWidth()
			sess.focus = FocusRightPanel
			sess.input.Blur()
		}
	case "clear":
		if sess != nil && sess.client != nil {
			sess.client.SendCancel()
		}
		if sess != nil {
			m.flushSessionBuf(sess)
			sess.chatMessages = nil
		}
	case "copy_conversation":
		if sess == nil || len(sess.chatMessages) == 0 {
			if sess != nil {
				sess.chatMessages = append(sess.chatMessages, renderSystemMessage("No conversation to copy.", m.styles))
			}
		} else {
			text := formatConversationPlainText(sess.chatMessages)
			count := len(sess.chatMessages)
			if cmd := copyToClipboardCmd(text); cmd != nil {
				cmds = append(cmds, cmd)
			}
			sess.chatMessages = append(sess.chatMessages, renderSystemMessage(fmt.Sprintf("Copied %d messages to clipboard.", count), m.styles))
		}
	case "slash_clear":
		if sess != nil && sess.client != nil {
			sess.chatMessages = append(sess.chatMessages, renderUserMessage("/clear", m.mdRenderer.width))
			sess.chatScrollOffset = 0
			sess.agentState = StateStreaming
			cmds = append(cmds, sess.thinkingAnim.Start())
			sess.client.SendInput("/clear", nil)
		}
	case "slash_goal":
		cmds = append(cmds, m.handleGoalCommand(sess, arg)...)
	case "open_effort_picker":
		cmds = append(cmds, m.openEffortPicker(sess)...)
	case "open_workflow_picker":
		m.workflowPicker.Open(slashCommandArgs(arg, "workflow"))
		if sess != nil {
			sess.input.Blur()
		}

	case "history":
		if sess != nil && len(sess.history.entries) > 0 {
			sess.historyPanel.Open(len(sess.history.entries), m.height)
		}
	case "scroll_top":
		if sess != nil {
			sess.chatScrollOffset = m.sessionMaxScrollOffset(sess)
			// No longer need to force FocusChat; scroll keys + wheel
			// work on Chat tab unconditionally.
		}
	case "scroll_bottom":
		if sess != nil {
			sess.chatScrollOffset = 0
			// No longer need to force FocusChat.
		}
	case "toggle_thinking":
		if sess != nil {
			sess.showThinking = !sess.showThinking
			if sess.showThinking && sess.thinkingBuf != "" {
				sess.thinkingRendered = renderThinkingText(sess.thinkingBuf, m.styles, m.mdRenderer.width+4)
			} else {
				sess.thinkingRendered = ""
			}
			_ = config.SetShowThinking(sess.showThinking)
		}
	case "quit":
		if sess != nil && sess.client != nil {
			sess.client.SendCancel()
			sess.client.SendClose()
		}
		// Persist chat scrollback on quit so the user does not lose
		// any in-flight messages.
		m.persistSessions()
		cmds = append(cmds, tea.Quit)
	default:
		if strings.HasPrefix(action, "switch_tab_") {
			idxStr := strings.TrimPrefix(action, "switch_tab_")
			if i, err := strconv.Atoi(idxStr); err == nil {
				switch TabKind(i) {
				case TabKindSessions:
					m.activeTab = TabKindSessions
					m.syncSessionsSelected()
					cmds = append(cmds, m.sessionsInput.Focus())
				case TabKindChat:
					m.activeTab = TabKindChat
					if sess != nil {
						sess.unreadCount = 0
						cmds = append(cmds, sess.thinkingAnim.Resume())
					}
				case TabKindSettings:
					m.openSettingsTab()
				case TabKindWorkflows:
					m.activeTab = TabKindWorkflows
				}
			}
		}
	}
	return cmds
}
