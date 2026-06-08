package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/agents"
	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
	"github.com/Mateooo93/cortex-cli/internal/provider"
	"github.com/Mateooo93/cortex-cli/internal/subagent"
	"github.com/Mateooo93/cortex-cli/internal/tools"
)

// resolveSubagentModel picks the LLM for a sub-agent. An explicit spawn_agent
// model override wins; otherwise the parent session's active model is used.
// Agent definition defaults are intentionally ignored so sub-agents share the
// same provider credentials as the main chat.
func resolveSubagentModel(explicitOverride, sessionActive string) string {
	if strings.TrimSpace(explicitOverride) != "" {
		return strings.TrimSpace(explicitOverride)
	}
	return sessionActive
}

func (s *Session) loadAgentCatalog() map[string]agents.Agent {
	catalog := map[string]agents.Agent{}
	if defaults, err := config.DefaultAgents(); err == nil {
		for name, ag := range defaults {
			catalog[name] = ag
		}
	}
	paths := config.NewCortexPaths(s.configDir, config.HomeCortexDir(), s.workdir)
	if user, err := agents.LoadFromDirs(paths.Agents()); err == nil {
		for name, ag := range user {
			catalog[name] = ag
		}
	}
	return catalog
}

func (s *Session) handleSpawnAgent(call provider.ToolCall) *provider.Message {
	summary := summarizeArgs(call.Arguments)
	s.emitToolCall(call.ID, call.Name, call.Arguments, summary)

	task, _ := call.Arguments["task"].(string)
	if strings.TrimSpace(task) == "" {
		errMsg := "task is required"
		s.emitToolResult(call.ID, call.Name, "", true, errMsg, nil)
		return toolHistoryMessage(call.ID, call.Name, "", true, errMsg)
	}
	role, _ := call.Arguments["role"].(string)
	if strings.TrimSpace(role) == "" {
		role = "developer"
	}
	modelOverride, _ := call.Arguments["model"].(string)
	prompt, _ := call.Arguments["prompt"].(string)

	catalog := s.loadAgentCatalog()
	ag, ok := agents.Resolve(role, catalog)
	if !ok {
		ag = agents.Agent{
			Name:     "general",
			MaxTurns: 25,
		}
	}
	if strings.TrimSpace(prompt) != "" {
		ag.SystemPrompt = strings.TrimSpace(prompt)
		ag.Name = role
		if ag.Name == "" {
			ag.Name = "subagent"
		}
	}

	s.mu.Lock()
	sessionActive := s.active
	s.mu.Unlock()
	modelSpec := resolveSubagentModel(modelOverride, sessionActive)

	taskID := s.subagents.RegisterRunning(ag.Name, task)
	go s.runLocalSubagent(taskID, ag, task, modelSpec)

	out := fmt.Sprintf(
		"sub-agent dispatched: task_id=%s role=%s\n\nThe sub-agent is running in the background. Use task_output(task_id=%q) to check on it.",
		taskID, ag.Name, taskID,
	)
	s.emitToolResult(call.ID, call.Name, out, false, "", nil)
	return toolHistoryMessage(call.ID, call.Name, out, false, "")
}

func (s *Session) handleTaskOutput(call provider.ToolCall) *provider.Message {
	summary := summarizeArgs(call.Arguments)
	s.emitToolCall(call.ID, call.Name, call.Arguments, summary)

	taskID, _ := call.Arguments["task_id"].(string)
	if strings.TrimSpace(taskID) == "" {
		errMsg := "task_id is required"
		s.emitToolResult(call.ID, call.Name, "", true, errMsg, nil)
		return toolHistoryMessage(call.ID, call.Name, "", true, errMsg)
	}
	wait := false
	if v, ok := call.Arguments["wait"].(bool); ok {
		wait = v
	}

	if wait {
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			if ag, ok := s.subagents.Get(taskID); ok && ag.Status != subagent.StatusRunning {
				return s.finishTaskOutput(call, ag)
			}
			select {
			case <-s.done:
				errMsg := "session closed"
				s.emitToolResult(call.ID, call.Name, "", true, errMsg, nil)
				return toolHistoryMessage(call.ID, call.Name, "", true, errMsg)
			case <-time.After(250 * time.Millisecond):
			}
		}
	}

	ag, ok := s.subagents.Get(taskID)
	if !ok {
		errMsg := "unknown task_id: " + taskID
		s.emitToolResult(call.ID, call.Name, "", true, errMsg, nil)
		return toolHistoryMessage(call.ID, call.Name, "", true, errMsg)
	}
	return s.finishTaskOutput(call, ag)
}

func (s *Session) finishTaskOutput(call provider.ToolCall, ag subagent.Subagent) *provider.Message {
	var out string
	switch ag.Status {
	case subagent.StatusRunning:
		out = fmt.Sprintf("task_id=%s status=running role=%s\n\ntask: %s\n\nThe sub-agent is still working. Call task_output again later or use wait=true.", ag.ID, ag.Role, ag.Task)
	case subagent.StatusDone:
		out = fmt.Sprintf("task_id=%s status=done role=%s\n\n%s", ag.ID, ag.Role, ag.Output)
	case subagent.StatusFailed:
		out = fmt.Sprintf("task_id=%s status=failed role=%s\n\nerror: %s", ag.ID, ag.Role, ag.Error)
	default:
		out = fmt.Sprintf("task_id=%s status=unknown", ag.ID)
	}
	s.emitToolResult(call.ID, call.Name, out, ag.Status == subagent.StatusFailed, "", nil)
	isErr := ag.Status == subagent.StatusFailed
	return toolHistoryMessage(call.ID, call.Name, out, isErr, ag.Error)
}

func (s *Session) runLocalSubagent(taskID string, ag agents.Agent, task, modelSpec string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		select {
		case <-s.done:
			cancel()
		case <-ctx.Done():
		}
	}()

	toolReg := tools.NewFilteredRegistry(filterSubagentTools(ag.Tools))
	sys := agents.FormatSystemPrompt(ag.SystemPrompt, s.workdir)
	if sys == "" {
		sys = fmt.Sprintf("You are a %s sub-agent working in %s.", ag.Name, s.workdir)
	}
	sys += "\n\nComplete the assigned task, then reply with a concise final summary."

	history := []provider.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: task},
	}

	var finalOutput string
	var runErr error

	for turn := 0; turn < ag.MaxTurns; turn++ {
		if ctx.Err() != nil {
			runErr = ctx.Err()
			break
		}
		resp, err := s.callSubagentProvider(ctx, modelSpec, history, toolReg)
		if err != nil {
			runErr = err
			break
		}

		inlineCalls := extractInlineToolCalls(resp.Content)
		allCalls := append([]provider.ToolCall{}, resp.ToolCalls...)
		allCalls = append(allCalls, inlineCalls...)
		if len(allCalls) == 0 {
			finalOutput = strings.TrimSpace(resp.Content)
			break
		}

		history = append(history, provider.Message{
			Role:      "assistant",
			Content:   stripToolCallBlocks(resp.Content),
			ToolCalls: allCalls,
		})

		for _, tc := range allCalls {
			if ctx.Err() != nil {
				runErr = ctx.Err()
				break
			}
			msg := s.runSubagentToolCall(ctx, toolReg, tc)
			if msg != nil {
				history = append(history, *msg)
			}
		}
		if runErr != nil {
			break
		}
	}

	if runErr != nil {
		s.subagents.MarkFailed(taskID, runErr.Error())
		return
	}
	if finalOutput == "" {
		finalOutput = "(sub-agent finished without a text response)"
	}
	s.subagents.MarkDone(taskID, finalOutput)
}

func filterSubagentTools(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		switch n {
		case "spawn_agent", "task_output":
			continue
		default:
			out = append(out, n)
		}
	}
	return out
}

func (s *Session) runSubagentToolCall(ctx context.Context, reg *tools.Registry, call provider.ToolCall) *provider.Message {
	if ctx.Err() != nil {
		return nil
	}
	switch call.Name {
	case "todo_write", "ask_user_question", "spawn_agent", "task_output":
		return toolHistoryMessage(call.ID, call.Name, "", true, call.Name+" is not available to sub-agents")
	}
	tool, ok := reg.Get(call.Name)
	if !ok {
		return toolHistoryMessage(call.ID, call.Name, "", true, "unknown tool")
	}
	if raw, ok := call.Arguments["_raw"].(string); ok {
		call.Arguments = recoverArgsFromRaw(call.Name, call.Arguments, raw)
	}
	tctx := tools.Context{
		CWD:        s.workdir,
		AllowShell: s.cfg.Tools.AllowShell,
		AllowWrite: s.cfg.Tools.AllowWrite,
		AllowGit:   s.cfg.Tools.AllowGit,
		Processes:  s.processes,
	}
	res, _ := tool.Run(tctx, call.Arguments)
	if !res.OK {
		return toolHistoryMessage(call.ID, call.Name, res.Output, true, res.Error)
	}
	return toolHistoryMessage(call.ID, call.Name, res.Output, false, "")
}

func (s *Session) callSubagentProvider(ctx context.Context, modelSpec string, messages []provider.Message, reg *tools.Registry) (provider.Response, error) {
	canonical, mc, err := s.cfg.GetModel(modelSpec)
	if err != nil {
		s.mu.Lock()
		_, mc, err = s.cfg.GetModel(s.active)
		canonical = s.active
		s.mu.Unlock()
		if err != nil {
			return provider.Response{}, err
		}
	}
	prov, err := provider.New(provider.ModelConfig{
		Provider:         mc.Provider,
		Model:            mc.Model,
		BaseURL:          mc.BaseURL,
		APIKey:           s.resolveAPIKey(mc),
		Temperature:      mc.Temperature,
		MaxTokens:        mc.MaxTokens,
		CortexPromptMode: mc.CortexPromptMode,
	})
	if err != nil {
		return provider.Response{}, err
	}

	sessionEffort := s.ReasoningEffort()

	requestModel := mc.Model
	if canonical != "" && canonical != mc.Provider {
		requestModel = strings.TrimPrefix(canonical, mc.Provider+"/")
	}
	if strings.EqualFold(mc.Provider, "opengateway") {
		requestModel = s.opengatewayScopedModel(requestModel)
	}

	req := provider.Request{
		Model:            requestModel,
		Messages:         messages,
		Tools:            convertToolsToProvider(reg),
		ToolChoice:       provider.ToolChoice{Mode: "auto"},
		Temperature:      mc.Temperature,
		MaxTokens:        mc.MaxTokens,
		Stream:           true,
		ReasoningEffort:  provider.RequestReasoningEffort(mc.Provider, requestModel, sessionEffort),
		CortexPromptMode: mc.CortexPromptMode,
	}
	req.Messages = stripOrphanToolResults(req.Messages)
	return prov.Stream(ctx, req, func(provider.Chunk) {})
}

func (s *Session) emitLocalSubagents(list []subagent.Subagent) {
	items := make([]protocol.LocalSubagentItem, 0, len(list))
	for _, a := range list {
		item := protocol.LocalSubagentItem{
			ID:        a.ID,
			Role:      a.Role,
			Task:      a.Task,
			Status:    protocol.LocalSubagentStatus(a.Status),
			Output:    a.Output,
			Error:     a.Error,
			StartedAt: a.StartedAt.Unix(),
		}
		if !a.EndedAt.IsZero() {
			item.EndedAt = a.EndedAt.Unix()
		}
		items = append(items, item)
	}
	s.safeEmit(protocol.SessionEvent{
		Type: "local_subagents_updated",
		Data: protocol.EventLocalSubagentsUpdated{Subagents: items},
	})
}