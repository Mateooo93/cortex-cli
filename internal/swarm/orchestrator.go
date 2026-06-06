// Package swarm implements the multi-agent orchestrator. A goal is broken
// into tasks by a planner, the tasks are executed by role-specialist
// agents, and a final synthesis is produced.
package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/provider"
	"github.com/Mateooo93/cortex-cli/internal/tools"
	"github.com/google/uuid"
)

// Role is one specialist persona the orchestrator can dispatch to.
type Role struct {
	Name         string
	Description  string
	SystemPrompt string
}

// Roles mirrors the original TypeScript implementation.
var Roles = []Role{
	{
		Name:        "planner",
		Description: "Breaks goals into tasks, assigns work, and decides when the job is done.",
		SystemPrompt: "You are a planning agent. Analyze the user's goal, break it into small, concrete subtasks, " +
			"and assign them to specialist agents. You do not write code; you produce plans, checklists, " +
			"and coordination notes. Be concise. When all tasks are complete, produce a final summary.",
	},
	{
		Name:        "developer",
		Description: "Writes, edits, and refactors code. Prefers one focused patch per task.",
		SystemPrompt: "You are a developer agent. Write clean, correct code. Prefer a single focused patch per task. " +
			"Include necessary imports. Do not leave placeholders. Respect existing code style. " +
			"If you need more context, ask for a specific file or function.",
	},
	{
		Name:        "reviewer",
		Description: "Reviews code for bugs, style, security, and correctness.",
		SystemPrompt: "You are a reviewer agent. Review the provided code or plan. Report bugs, missing edge cases, " +
			"style issues, and security risks. Suggest concrete fixes. Be critical but constructive. " +
			"Output a concise review list.",
	},
	{
		Name:        "tester",
		Description: "Writes tests, runs them, and reports pass/fail.",
		SystemPrompt: "You are a tester agent. Write unit or integration tests for the provided code. " +
			"Run them if you have shell access. Report which tests pass and which fail, with the failure message. " +
			"Keep tests minimal but meaningful.",
	},
	{
		Name:        "researcher",
		Description: "Gathers documentation, API references, and implementation ideas.",
		SystemPrompt: "You are a researcher agent. Search documentation, read files, and summarize findings. " +
			"Provide sources or filenames. Do not write implementation code unless explicitly asked.",
	},
	{
		Name:        "fixer",
		Description: "Applies fixes suggested by reviewer or tester.",
		SystemPrompt: "You are a fixer agent. Take review comments or test failures and apply the minimal, correct fix. " +
			"Preserve surrounding code. Write only the changed code, not the whole file, unless asked.",
	},
	{
		Name:        "documenter",
		Description: "Writes README, API docs, or inline comments.",
		SystemPrompt: "You are a documenter agent. Write clear documentation: README sections, API docs, or inline comments. " +
			"Match the project's tone. Include examples. Keep it concise.",
	},
}

// RolesForStrategy returns the role list appropriate for a given strategy.
func RolesForStrategy(strategy string) []Role {
	switch strategy {
	case "development":
		return []Role{Roles[0], Roles[1], Roles[2], Roles[3], Roles[4]}
	case "research":
		return []Role{Roles[0], Roles[5], Roles[6]}
	case "testing":
		return []Role{Roles[0], Roles[3], Roles[2]}
	case "optimization":
		return []Role{Roles[0], Roles[1], Roles[3], Roles[2]}
	default:
		return []Role{Roles[0], Roles[1], Roles[2]}
	}
}

// Goal is a high-level description of work to do.
type Goal struct {
	Title        string
	Description  string
	Deliverables []string
	Strategy     string
	Mode         string
	MaxAgents    int
	Timeout      int
}

// Task is a single unit of work assigned to one role.
type Task struct {
	ID          string
	Description string
	AssignedTo  string
	DependsOn   []string
	Status      string // "pending" | "in_progress" | "done" | "failed"
	Output      string
}

// Hooks let the caller observe orchestrator events for UI integration.
type Hooks struct {
	OnLog       func(level, text string)
	OnPhase     func(phase, detail string)
	OnTaskStart func(id, role, desc string)
	OnTaskDone  func(id, role, output string)
}

// Orchestrator runs a Goal.
type Orchestrator struct {
	cfg   *cortexconfig.Config
	model string
	hooks Hooks
	tools *tools.Registry
}

// New constructs an Orchestrator.
func New(cfg *cortexconfig.Config, model string) *Orchestrator {
	if model == "" {
		model = cfg.DefaultModel
	}
	return &Orchestrator{cfg: cfg, model: model, tools: tools.NewRegistry()}
}

// SetHooks registers UI integration callbacks.
func (o *Orchestrator) SetHooks(h Hooks) { o.hooks = h }

func (o *Orchestrator) log(level, text string) {
	if o.hooks.OnLog != nil {
		o.hooks.OnLog(level, text)
		return
	}
	fmt.Println(text)
}

func (o *Orchestrator) phase(phase, detail string) {
	if o.hooks.OnPhase != nil {
		o.hooks.OnPhase(phase, detail)
	}
}

func (o *Orchestrator) taskStart(id, role, desc string) {
	if o.hooks.OnTaskStart != nil {
		o.hooks.OnTaskStart(id, role, desc)
	}
}

func (o *Orchestrator) taskDone(id, role, output string) {
	if o.hooks.OnTaskDone != nil {
		o.hooks.OnTaskDone(id, role, output)
	}
}

// Run executes the goal: plan → execute tasks (sequential in centralized
// mode, parallel otherwise) → synthesize. The synthesis text is returned.
func (o *Orchestrator) Run(ctx context.Context, g Goal) (string, error) {
	o.log("info", "\n◉ Goal: "+g.Title)
	o.log("info", g.Description)
	o.log("info", fmt.Sprintf("Strategy: %s · Mode: %s · Agents: %d\n", g.Strategy, g.Mode, g.MaxAgents))

	roles := RolesForStrategy(g.Strategy)
	if g.MaxAgents > 0 && g.MaxAgents < len(roles) {
		roles = roles[:g.MaxAgents]
	}

	o.phase("planning", "Generating task graph")
	planner := roles[0]
	tasks, overview, err := o.runPlanner(ctx, planner, roles, g)
	if err != nil {
		return "", err
	}
	o.log("info", "◉ Planner: "+overview)
	o.log("info", fmt.Sprintf("   %d task(s) planned.\n", len(tasks)))

	o.phase("executing", fmt.Sprintf("%d task(s)", len(tasks)))
	if g.Mode == "centralized" || g.Mode == "" {
		for _, t := range tasks {
			if err := o.executeTask(ctx, t, roles, g); err != nil {
				return "", err
			}
		}
	} else {
		o.runParallel(ctx, tasks, roles, g)
	}

	o.phase("synthesizing", "")
	final, err := o.synthesize(ctx, planner, tasks, g)
	if err != nil {
		return "", err
	}
	o.log("ok", "\n◉ Goal complete\n")
	o.log("info", final)
	return final, nil
}

func (o *Orchestrator) runPlanner(ctx context.Context, planner Role, roles []Role, g Goal) ([]Task, string, error) {
	prompt := fmt.Sprintf(`Goal: %s
Description: %s
Deliverables: %s
Strategy: %s
Available agents: %s

Break this into concrete tasks. Each task should have:
- id (task_1, task_2, ...)
- description (what to do)
- assignedTo (which agent role handles it)
- dependsOn (ids of tasks that must finish first, if any)

Output ONLY a JSON object in this exact schema (no markdown, no prose):
{"tasks": [{"id":"task_1","description":"...","assignedTo":"developer","dependsOn":[]}], "overview":"Short plan summary"}`,
		g.Title, g.Description, strings.Join(g.Deliverables, ", "), g.Strategy, joinNames(roles),
	)
	resp, err := o.callRole(ctx, planner, prompt, g)
	if err != nil {
		return nil, "", err
	}
	tasks, overview, err := parsePlannerResponse(resp)
	if err != nil {
		// Fallback: one big developer task
		tasks = []Task{{
			ID:          "task_1",
			Description: g.Description,
			AssignedTo:  "developer",
			Status:      "pending",
		}}
		overview = resp
	}
	return tasks, overview, nil
}

func (o *Orchestrator) executeTask(ctx context.Context, t Task, roles []Role, g Goal) error {
	role := findRole(roles, t.AssignedTo)
	if role == nil {
		o.log("err", "✖ Unknown role: "+t.AssignedTo)
		return nil
	}
	t.Status = "in_progress"
	o.log("warn", fmt.Sprintf("▸ %s [%s]: %s", t.ID, role.Name, t.Description))
	o.taskStart(t.ID, role.Name, t.Description)

	depCtx := o.dependencyContext(t)
	prompt := fmt.Sprintf("Goal: %s\n%s\n\nYour task: %s\n%s\n\nComplete this task. Output your result directly. Be concise.",
		g.Title, g.Description, t.Description, depCtx,
	)
	resp, err := o.callRole(ctx, *role, prompt, g)
	if err != nil {
		t.Status = "failed"
		return err
	}
	t.Output = resp
	t.Status = "done"
	o.taskDone(t.ID, role.Name, resp)
	o.log("ok", fmt.Sprintf("✓ %s done (%d chars)", t.ID, len(resp)))
	return nil
}

func (o *Orchestrator) dependencyContext(t Task) string {
	if len(t.DependsOn) == 0 {
		return ""
	}
	// The caller passes the full task list to the orchestrator; we
	// can't access it here. For simplicity we leave a marker and let
	// the synthesis step do the cross-task stitching. (In v0.1, tasks
	// are mostly sequential anyway.)
	return fmt.Sprintf("\nDepends on: %s\n", strings.Join(t.DependsOn, ", "))
}

func (o *Orchestrator) runParallel(ctx context.Context, tasks []Task, roles []Role, g Goal) {
	done := map[string]bool{}
	safety := len(tasks)*2 + 1
	for len(done) < len(tasks) && safety > 0 {
		safety--
		ready := []Task{}
		for _, t := range tasks {
			if t.Status == "pending" && depsSatisfied(t, done) {
				ready = append(ready, t)
			}
		}
		if len(ready) == 0 {
			break
		}
		for _, t := range ready {
			_ = o.executeTask(ctx, t, roles, g)
			done[t.ID] = true
		}
	}
}

func depsSatisfied(t Task, done map[string]bool) bool {
	for _, d := range t.DependsOn {
		if !done[d] {
			return false
		}
	}
	return true
}

func (o *Orchestrator) synthesize(ctx context.Context, planner Role, tasks []Task, g Goal) (string, error) {
	var b strings.Builder
	for _, t := range tasks {
		b.WriteString("## task_" + t.ID + " (" + t.AssignedTo + ")\n")
		b.WriteString(t.Output)
		b.WriteString("\n\n")
	}
	prompt := fmt.Sprintf("Goal: %s\n\nAll agent outputs:\n%s\n\nSynthesize a final, coherent deliverable. Address the original deliverables: %s. Be concise and actionable.",
		g.Title, b.String(), strings.Join(g.Deliverables, ", "),
	)
	return o.callRole(ctx, planner, prompt, g)
}

// callRole invokes the model with the role's system prompt + the user prompt.
func (o *Orchestrator) callRole(ctx context.Context, role Role, userPrompt string, g Goal) (string, error) {
	_, mc, err := o.cfg.GetModel(o.model)
	if err != nil {
		return "", err
	}
	prov, err := provider.New(provider.ModelConfig{
		Provider:    mc.Provider,
		Model:       mc.Model,
		BaseURL:     mc.BaseURL,
		APIKey:      resolveRoleAPIKey(mc),
		Temperature: mc.Temperature,
		MaxTokens:   mc.MaxTokens,
	})
	if err != nil {
		return "", err
	}
	req := provider.Request{
		Model: mc.Model,
		Messages: []provider.Message{
			{Role: "system", Content: role.SystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream:      false,
		Temperature: mc.Temperature,
		MaxTokens:   mc.MaxTokens,
	}
	resp, err := prov.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func resolveRoleAPIKey(mc *cortexconfig.ModelConfig) string {
	if mc.APIKey != "" {
		return mc.APIKey
	}
	if envVar := cortexconfig.ProviderEnvVar(mc.Provider); envVar != "" {
		return os.Getenv(envVar)
	}
	return ""
}

// parsePlannerResponse extracts a JSON plan from the planner's output.
// Tries fenced ```json first, then braced top-level.
var fencedJSONRE = regexp.MustCompile("(?s)```(?:json)?\\s*\\n([\\s\\S]*?)\\n```")

func parsePlannerResponse(content string) ([]Task, string, error) {
	var jsonStr string
	if m := fencedJSONRE.FindStringSubmatch(content); m != nil {
		jsonStr = m[1]
	} else {
		// Find first balanced { ... }
		start := strings.Index(content, "{")
		if start < 0 {
			return nil, "", fmt.Errorf("no JSON found in planner response")
		}
		depth, inString, escape := 0, false, false
		for i := start; i < len(content); i++ {
			c := content[i]
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = !inString
				continue
			}
			if inString {
				continue
			}
			if c == '{' {
				depth++
			} else if c == '}' {
				depth--
				if depth == 0 {
					jsonStr = content[start : i+1]
					break
				}
			}
		}
		if jsonStr == "" {
			return nil, "", fmt.Errorf("unbalanced JSON")
		}
	}
	var parsed struct {
		Tasks []struct {
			ID          string   `json:"id"`
			Description string   `json:"description"`
			AssignedTo  string   `json:"assignedTo"`
			DependsOn   []string `json:"dependsOn"`
		} `json:"tasks"`
		Overview string `json:"overview"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, "", err
	}
	out := make([]Task, 0, len(parsed.Tasks))
	for _, t := range parsed.Tasks {
		out = append(out, Task{
			ID:          t.ID,
			Description: t.Description,
			AssignedTo:  t.AssignedTo,
			DependsOn:   t.DependsOn,
			Status:      "pending",
		})
	}
	return out, parsed.Overview, nil
}

func findRole(roles []Role, name string) *Role {
	for i, r := range roles {
		if r.Name == name {
			return &roles[i]
		}
	}
	return nil
}

func joinNames(roles []Role) string {
	names := make([]string, len(roles))
	for i, r := range roles {
		names[i] = r.Name
	}
	return strings.Join(names, ", ")
}

// Avoid unused import (uuid might be used in future).
var _ = uuid.NewString
