package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/protocol"
)

func (m *Model) applyEventToSession(idx int, event protocol.SessionEvent) []tea.Cmd {
	sess := m.sessions[idx]
	var cmds []tea.Cmd

	switch event.Type {
	case "event.session_started":
		data := marshalData(event.Data)
		var started protocol.EventSessionStarted
		json.Unmarshal(data, &started)
		sess.parentID = started.ParentID
		sess.forkTurnIdx = started.ForkTurnIdx

	case "event.init_state":
		data := marshalData(event.Data)
		var state protocol.EventInitState
		json.Unmarshal(data, &state)
		sess.initState = protocol.InitState(state.State)
		if state.Model != "" {
			sess.modelName = m.canonicalSettingsModel(state.Model)
		}

	case "event.stream_chunk":
		data := marshalData(event.Data)
		var chunk protocol.EventStreamChunk
		json.Unmarshal(data, &chunk)
		if chunk.Text == "" {
			break
		}
		sess.streamPending += chunk.Text
		cmds = append(cmds, sess.streamPlayback.EnsureTick())

	case "event.thinking_chunk":
		data := marshalData(event.Data)
		var chunk protocol.EventThinkingChunk
		json.Unmarshal(data, &chunk)
		sess.thinkingBuf += chunk.Text
		if sess.showThinking {
			sess.thinkingRendered = renderThinkingText(sess.thinkingBuf, m.styles, m.mdRenderer.width+4)
		}

	case "event.stream_done":
		data := marshalData(event.Data)
		var done protocol.EventStreamDone
		json.Unmarshal(data, &done)
		flushStreamPlayback(sess)
		sess.streamPlayback.Stop()
		sess.chatScrollOffset = 0
		if sess.assistantBuf != "" {
			sess.assistantRendered = strings.TrimLeft(m.mdRenderer.Render(sess.assistantBuf), "\n")
			sess.streamCache.reset()
		}
		// Context-window counting fix. The streaming API
		// reports `InputTokens` as the prompt size of the
		// CURRENT turn (which includes the entire
		// conversation history). It is NOT the number of
		// new tokens this turn. Accumulating per-turn
		// prompt tokens (the previous behaviour) gave
		// double / triple counting: 10 turns × 1000
		// tokens each summed to 10000 even though the
		// actual context was 10000 only on the last
		// turn. The status bar then showed the context
		// filling up "abnormally quickly" and triggered
		// auto-compact way too early.
		//
		// Correct model: the most recent non-zero
		// prompt-size report is the most accurate
		// representation of the current context. We
		// accept any report that's larger than the
		// current (normal growth) AND any report that's
		// smaller (compaction / context-reset / new
		// session). We IGNORE 0 (some providers don't
		// report prompt tokens on every chunk).
		if done.InputTokens > 0 {
			sess.inputTokens = done.InputTokens
		}
		// Output tokens ARE additive across turns (the
		// model emits new tokens each turn, never re-reads
		// old output), so accumulate them.
		sess.outputTokens += done.OutputTokens
		turnElapsed := sess.FinishTurn()
		if done.ElapsedMs > 0 {
			sess.elapsed = time.Duration(done.ElapsedMs) * time.Millisecond
		} else if turnElapsed > 0 {
			sess.elapsed = turnElapsed
		}
		sess.lastTurnInputTokens = done.InputTokens
		sess.lastTurnOutputTokens = done.OutputTokens
		sess.lastTurnCacheCreate = done.CacheCreationTokens
		sess.lastTurnCacheRead = done.CacheReadTokens
		sess.lastOutputTokens = done.OutputTokens
		if strings.EqualFold(done.FinishReason, "length") {
			cmds = append(cmds, m.emitStatusMsg("response hit max output tokens and was cut off — continuing with higher maxTokens or ask 'continue'", StatusMsgWarning))
		}
		// After every assistant turn, check whether the
		// session is close to its context window. If
		// auto-compact is enabled and usage >= 80%, kick
		// off a compaction in the background so the next
		// turn starts with a clean context.
		if autoCmd := m.maybeAutoCompact(); autoCmd != nil {
			cmds = append(cmds, autoCmd)
		}

	case "event.tool_call":
		m.flushSessionBuf(sess)
		sess.agentState = StateToolExecuting
		data := marshalData(event.Data)
		var tc protocol.EventToolCall
		json.Unmarshal(data, &tc)
		bashReasons := [4]string{tc.ReasonNotReadFile, tc.ReasonNotEditFile, tc.ReasonNotGlobFiles, tc.ReasonToIncreaseTimeout}
		chatIdx := len(sess.chatMessages)
		msg := renderToolCall(tc.Name, tc.Summary, tc.Reason, bashReasons, m.styles)
		if isShellTool(tc.Name) {
			now := time.Now()
			msg.ToolID = tc.ToolID
			msg.ToolStatus = ToolRunPending
			msg.ToolReason = tc.Reason
			msg.ToolBashReasons = bashReasons
			msg.StartedAt = now
			msg.Rendered = renderShellToolCallDisplay(tc.Name, tc.Summary, tc.Reason, bashReasons, ToolRunPending, now, time.Time{}, sess.toolActivityAnim.Frame(), m.styles)
		}
		sess.chatMessages = append(sess.chatMessages, msg)
		if tc.ToolID != "" {
			if sess.pendingTools == nil {
				sess.pendingTools = make(map[string]int)
			}
			sess.pendingTools[tc.ToolID] = chatIdx
		}
		if !isOrchestrationTool(tc.Name) {
			sess.pushRecentTool(RecentToolEntry{
				ToolID:    tc.ToolID,
				Name:      tc.Name,
				Summary:   tc.Summary,
				StartedAt: time.Now(),
				Status:    RecentToolPending,
			})
		}
		if cmd := sess.toolActivityAnim.Start(); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case "event.tool_result":
		data := marshalData(event.Data)
		var tr protocol.EventToolResult
		json.Unmarshal(data, &tr)
		detail := tr.Detail
		if sess.confirmDetailShown && tr.Name == sess.confirmToolName {
			detail = ""
			sess.confirmDetailShown = false
		}
		var toolCallSummary string
		if tr.ToolID != "" && sess.pendingTools != nil {
			if callIdx, ok := sess.pendingTools[tr.ToolID]; ok && callIdx < len(sess.chatMessages) {
				call := &sess.chatMessages[callIdx]
				toolCallSummary = call.Text
				if isShellTool(call.ToolName) && call.ToolStatus == ToolRunPending {
					ended := time.Now()
					status := ToolRunDone
					if tr.IsError {
						status = ToolRunFailed
					}
					call.ToolStatus = status
					call.EndedAt = ended
					call.Rendered = renderShellToolCallDisplay(call.ToolName, call.Text, call.ToolReason, call.ToolBashReasons, status, call.StartedAt, ended, 0, m.styles)
				}
			}
		}
		result := renderToolResultWithContext(tr.Name, tr.Output, tr.IsError, tr.ShowToolName, detail, toolCallSummary, m.styles, m.mdRenderer, m.mdRenderer.width)

		if tr.ToolID != "" {
			sess.markRecentToolDone(tr.ToolID, tr.IsError)
			if !sess.hasPendingRecentTools() {
				sess.toolActivityAnim.Stop()
			}
		}

		if tr.ToolID != "" && sess.pendingTools != nil {
			if callIdx, ok := sess.pendingTools[tr.ToolID]; ok {
				insertIdx := callIdx + 1
				delete(sess.pendingTools, tr.ToolID)
				if insertIdx <= len(sess.chatMessages) {
					sess.chatMessages = append(sess.chatMessages, ChatMessage{})
					copy(sess.chatMessages[insertIdx+1:], sess.chatMessages[insertIdx:])
					sess.chatMessages[insertIdx] = result
					for id, idx2 := range sess.pendingTools {
						if idx2 >= insertIdx {
							sess.pendingTools[id] = idx2 + 1
						}
					}
				} else {
					sess.chatMessages = append(sess.chatMessages, result)
				}
			} else {
				sess.chatMessages = append(sess.chatMessages, result)
			}
		} else {
			sess.chatMessages = append(sess.chatMessages, result)
		}

	case "event.confirm_request":
		sess.agentState = StateConfirmPending
		data := marshalData(event.Data)
		var cr protocol.EventConfirmRequest
		json.Unmarshal(data, &cr)
		sess.confirmToolName = cr.ToolName
		sess.confirmDetailShown = false
		sess.thinkingAnim.Stop()
		if cr.Detail != "" {
			sess.chatMessages = append(sess.chatMessages,
				renderToolResultWithContext(cr.ToolName, "", false, false, cr.Detail, "", m.styles, m.mdRenderer, m.mdRenderer.width))
			sess.confirmDetailShown = true
		}
		question := buildConfirmQuestion(cr.ToolName, cr.Params)
		if len(cr.RequestedDirs) > 0 {
			question = buildDirAccessQuestion(cr.RequestedDirs)
		}
		sess.chatMessages = append(sess.chatMessages,
			renderQuestionMessage("Permission", question, m.mdRenderer.width+4, m.mdRenderer))
		sess.questionPanel.OpenConfirm(cr.ToolName, cr.Params, cr.RequestedDirs, m.width, m.mdRenderer)
		sess.focus = FocusEditor

	case "event.user_question":
		data := marshalData(event.Data)
		var uq protocol.EventUserQuestion
		json.Unmarshal(data, &uq)
		sess.questionPanel.Open(uq, m.width, m.mdRenderer)
		sess.agentState = StateUserQuestion
		sess.thinkingAnim.Stop()
		sess.focus = FocusEditor
		sess.input.Blur()

	case "event.background_processes_updated":
		data := marshalData(event.Data)
		var bp protocol.EventBackgroundProcessesUpdated
		json.Unmarshal(data, &bp)
		sess.backgroundProcesses = bp.Processes
		if len(runningBackgroundProcesses(bp.Processes)) > 0 {
			if !sess.rightPanel.IsVisible() {
				sess.rightPanel.OpenInfo(m.height)
				m.updateChatWidth()
			}
			if cmd := sess.toolActivityAnim.Start(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		} else {
			sess.toolActivityAnim.Stop()
		}

	case "event.local_subagents_updated":
		data := marshalData(event.Data)
		var ls protocol.EventLocalSubagentsUpdated
		json.Unmarshal(data, &ls)
		sess.localSubagents = ls.Subagents
		if len(runningLocalSubagents(ls.Subagents)) > 0 {
			if !sess.rightPanel.IsVisible() {
				sess.rightPanel.OpenInfo(m.height)
				m.updateChatWidth()
			}
		}

	case "event.todo_list_updated":
		data := marshalData(event.Data)
		var tu protocol.EventTodoListUpdated
		json.Unmarshal(data, &tu)
		sess.todos = tu.Todos
		switch sess.rightPanel.mode {
		case rpModeTodos:
			if !hasPendingTodos(sess.todos) {
				sess.rightPanel.Close()
				m.updateChatWidth()
			}
		default:
			if !sess.rightPanel.IsVisible() && hasPendingTodos(sess.todos) {
				sess.rightPanel.OpenTodos(m.height)
				m.updateChatWidth()
			}
		}

	case "event.plan_proposed":
		data := marshalData(event.Data)
		var pp protocol.EventPlanProposed
		json.Unmarshal(data, &pp)
		sess.activePlan = pp.Plan
		sess.agentState = StatePlanReview
		sess.chatMessages = append(sess.chatMessages, renderPlanProposal(pp.Plan, m.styles))
		sess.input.Focus()
		sess.input.Placeholder = "Type modifications or press y/n..."

	case "event.plan_task_start":
		sess.agentState = StatePlanExecuting
		data := marshalData(event.Data)
		var pts protocol.EventPlanTaskStart
		json.Unmarshal(data, &pts)
		sess.chatMessages = append(sess.chatMessages, renderPlanTaskStart(pts.TaskIdx, pts.Title, pts.Total))
		cmds = append(cmds, sess.thinkingAnim.Start())

	case "event.plan_task_done":
		sess.thinkingAnim.Stop()
		data := marshalData(event.Data)
		var ptd protocol.EventPlanTaskDone
		json.Unmarshal(data, &ptd)
		sess.chatMessages = append(sess.chatMessages, renderPlanTaskDone(ptd.TaskIdx, ptd.Title, ptd.Success, ptd.Summary, m.styles))

	case "event.plan_complete":
		data := marshalData(event.Data)
		var pc protocol.EventPlanComplete
		json.Unmarshal(data, &pc)
		sess.activePlan = nil
		sess.chatMessages = append(sess.chatMessages, renderPlanSummary(pc.Plan))

	case "event.agent_done":
		sess.thinkingAnim.Stop()
		m.flushSessionBuf(sess)
		if idx != m.selectedSession || m.activeTab != TabKindChat {
			sess.unreadCount++
		}
		pricingSpec := protocol.ResolvePricingSpec(sess.modelName, "", "")
		if m.cortexCfg != nil {
			if _, mc, err := m.cortexCfg.GetModel(sess.modelName); err == nil {
				pricingSpec = protocol.ResolvePricingSpec(sess.modelName, mc.Provider, mc.Model)
			}
		}
		turnInput := sess.lastTurnInputTokens
		turnOutput := sess.lastTurnOutputTokens
		if turnInput == 0 {
			turnInput = sess.inputTokens - sess.turnStartInputTokens
		}
		if turnOutput == 0 {
			turnOutput = sess.outputTokens - sess.turnStartOutputTokens
		}
		turnCacheCreation := sess.lastTurnCacheCreate
		if turnCacheCreation == 0 {
			turnCacheCreation = sess.cacheCreationTokens - sess.turnStartCacheCreationTokens
		}
		turnCacheRead := sess.lastTurnCacheRead
		if turnCacheRead == 0 {
			turnCacheRead = sess.cacheReadTokens - sess.turnStartCacheReadTokens
		}
		cost := protocol.CalculateCost(pricingSpec, turnInput, turnOutput, turnCacheCreation, turnCacheRead)
		sess.chatMessages = append(sess.chatMessages, renderTurnInfo(sess.modelName, sess.elapsed, cost, m.mdRenderer.width+4, m.styles))
		sess.turnStartInputTokens = sess.inputTokens
		sess.turnStartOutputTokens = sess.outputTokens
		sess.turnStartCacheCreationTokens = sess.cacheCreationTokens
		sess.turnStartCacheReadTokens = sess.cacheReadTokens
		// Persist chat scrollback to disk at the end of every turn
		// so an unexpected crash or kill does not lose the user's
		// most recent assistant response.
		m.persistSessions()
		if sess.pendingInput != nil {
			pending := sess.pendingInput
			sess.pendingInput = nil
			if sess.client != nil {
				sess.client.SendInput(pending.text, pending.attachments)
			}
			sess.agentState = StateStreaming
			// The pending message became a fresh user turn —
			// restart the per-turn timer.
			sess.StartTurn()
			// If another message is queued (e.g. the user
			// pressed Tab again while this turn was running),
			// keep showing the queued badge in the placeholder.
			sess.input.Placeholder = m.placeholderForMode(sess)
			cmds = append(cmds, sess.thinkingAnim.Start())
		} else {
			sess.agentState = StateWaitingForInput
			sess.input.Focus()
			sess.input.Placeholder = m.placeholderForMode(sess)
		}

	case "event.clear":
		m.flushSessionBuf(sess)
		sess.chatMessages = nil
		sess.pendingTools = nil
		sess.inputTokens = 0
		sess.outputTokens = 0
		sess.cacheCreationTokens = 0
		sess.cacheReadTokens = 0
		sess.turnStartInputTokens = 0
		sess.turnStartOutputTokens = 0
		sess.turnStartCacheCreationTokens = 0
		sess.turnStartCacheReadTokens = 0
		sess.elapsed = 0
		sess.chatMessages = append(sess.chatMessages, renderSystemMessage("Conversation cleared.", m.styles))
		// Persist the cleared scrollback.
		m.persistSessions()

	case "event.retry":
		data := marshalData(event.Data)
		var retry protocol.EventRetry
		json.Unmarshal(data, &retry)
		m.flushSessionBuf(sess)
		sess.chatMessages = append(sess.chatMessages, renderRetryMessage(retry))
		// Persist the retry notification.
		m.persistSessions()

	case "event.error":
		data := marshalData(event.Data)
		var errEvent protocol.EventError
		json.Unmarshal(data, &errEvent)
		sess.thinkingAnim.Stop()
		m.flushSessionBuf(sess)
		sess.pendingInput = nil
		sess.pendingPlanAction = nil
		sess.chatMessages = append(sess.chatMessages, renderErrorMessage(fmt.Errorf("%s", errEvent.Message)))
		if sess.agentState != StatePlanReview && sess.agentState != StateUserQuestion && sess.agentState != StateConfirmPending {
			sess.agentState = StateWaitingForInput
			sess.input.Focus()
			sess.input.Placeholder = m.placeholderForMode(sess)
		}
		// Persist the latest scrollback so the error message is
		// on disk in case the user kills the process before any
		// further event fires.
		m.persistSessions()

	case "event.quit":
		cmds = append(cmds, tea.Quit)
	}

	return cmds
}

// marshalData converts event.Data back to bytes.
func marshalData(data any) []byte {
	b, _ := json.Marshal(data)
	return b
}
