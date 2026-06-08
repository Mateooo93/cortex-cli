// Package workflow implements the multi-agent workflow engine that
// powers the Workflows tab and the /workflow slash command.
//
// A Workflow is a named execution plan that dispatches tasks to
// role-specialist agents (planner, developer, reviewer, tester,
// researcher, fixer, documenter). The engine runs in the same
// process as the TUI \u2014 there's no daemon involved \u2014 and emits
// events via a callback so the UI can update a live progress panel
// in real time.
//
// Three execution modes:
//
//   - local_subagent: one agent runs in a goroutine inside the
//     main process. The main agent stays interactive; the user
//     can keep chatting. Used by the `spawn_agent` tool.
//
//   - workflow:       multiple agents run sequentially or in
//     parallel, coordinated by a planner role. Used by the
//     /workflow slash command and the auto-detect heuristic.
//
//   - background:     same as workflow but the orchestrator runs
//     detached and the main agent can do other work. The user
//     sees the result when it's ready.
package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	llmprovider "github.com/Mateooo93/cortex-cli/internal/provider"
)

// Role is one specialist persona the orchestrator can dispatch to.
type Role struct {
	Name         string
	Description  string
	SystemPrompt string
}

// BuiltinRoles is the list of specialist roles every workflow
// can draw from. Mirrors swarm.Roles but kept separate so the
// workflow engine can evolve independently.
var BuiltinRoles = []Role{
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

// Preset is a canned workflow the user can pick from the Workflows
// tab or trigger via /workflow <name>. Each preset is a strategy
// (which roles to use) + a default max-agent count.
type Preset struct {
	Name        string
	Description string
	Strategy    string
	MaxAgents   int
	Roles       []string
}

// BuiltinPresets is the set of out-of-the-box workflows.
var BuiltinPresets = []Preset{
	{
		Name:        "code",
		Description: "Plan, implement, review, and test a coding task end-to-end.",
		Strategy:    "development",
		MaxAgents:   5,
		Roles:       []string{"planner", "developer", "reviewer", "tester", "researcher"},
	},
	{
		Name:        "research",
		Description: "Gather documentation and reference material, then summarise findings.",
		Strategy:    "research",
		MaxAgents:   3,
		Roles:       []string{"planner", "researcher", "documenter"},
	},
	{
		Name:        "test",
		Description: "Write and run tests for an existing code change.",
		Strategy:    "testing",
		MaxAgents:   4,
		Roles:       []string{"planner", "tester", "reviewer", "fixer"},
	},
	{
		Name:        "review",
		Description: "Review a diff or plan, surface issues, and suggest fixes.",
		Strategy:    "optimization",
		MaxAgents:   4,
		Roles:       []string{"planner", "reviewer", "fixer", "tester"},
	},
	{
		Name:        "docs",
		Description: "Write or improve project documentation (README, API docs, comments).",
		Strategy:    "research",
		MaxAgents:   3,
		Roles:       []string{"planner", "researcher", "documenter"},
	},
}

// Status of a single step in a workflow.
const (
	StepPending    = "pending"
	StepInProgress = "in_progress"
	StepDone       = "done"
	StepFailed     = "failed"
	StepCancelled  = "cancelled"
)

// Step is one task inside a workflow. The engine updates its
// Status as the agent progresses.
type Step struct {
	ID          string
	Description string
	Role        string
	Status      string
	Output      string
	CurrentMsg  string        // "what the agent is doing right now"
	StartedAt   time.Time
	EndedAt     time.Time
	Duration    time.Duration
}

// Snapshot is a point-in-time view of a workflow for the UI.
type Snapshot struct {
	ID           string
	Name         string
	Goal         string
	Status       string // "planning" | "running" | "synthesizing" | "done" | "failed" | "cancelled"
	StartedAt    time.Time
	EndedAt      time.Time
	Duration     time.Duration
	Steps        []Step
	Summary      string
	CurrentMsg   string // what the active step is currently doing
	DoneSteps    int    // count of steps whose Status == StepDone
	TotalSteps   int    // total steps the workflow will execute
	BudgetSpent  int64  // tokens spent in this run
	BudgetTotal  int64  // token cap (0 = unlimited)
}

// Engine runs workflows. The engine is goroutine-safe; multiple
// workflows can run concurrently (e.g. one orchestrated by the
// main agent, one dispatched as a background sub-agent).
type Engine struct {
	cfg     *cortexconfig.Config
	mu      sync.RWMutex
	flows   map[string]*Workflow
	hooks   Hooks
	cancel  map[string]context.CancelFunc
	journal *Journal        // per-run journal for resume support
	budget  *Budget         // token budget (may be nil = unlimited)
}

// Hooks is the callback set the UI registers. All fields are
// optional; nil means "don't notify for this event".
type Hooks struct {
	OnWorkflowStart    func(snap Snapshot)
	OnStepStart       func(workflowID, stepID string, snap Snapshot)
	OnStepProgress    func(workflowID, stepID, msg string, snap Snapshot)
	OnStepDone        func(workflowID, stepID string, snap Snapshot)
	OnWorkflowComplete func(snap Snapshot)
}

// Workflow is one execution instance. The TUI keeps a list of
// these in memory (no persistence yet \u2014 the user can re-run
// from the goal in the Workflows tab).
type Workflow struct {
	ID        string
	Name      string
	Goal      string
	Strategy  string
	MaxAgents int
	Status    string
	StartedAt time.Time
	EndedAt   time.Time
	Steps     []*Step
	Summary   string
	Budget    *Budget          // token budget for this run
	Journal   *Journal         // call journal for resume
	cancel    context.CancelFunc
	currentMsg atomic.Value // string
}

// NewEngine constructs an Engine bound to the user's config.
func NewEngine(cfg *cortexconfig.Config) *Engine {
	return &Engine{
		cfg:    cfg,
		flows:  map[string]*Workflow{},
		cancel: map[string]context.CancelFunc{},
	}
}

// SetHooks installs the UI callbacks.
func (e *Engine) SetHooks(h Hooks) { e.hooks = h }

// SetBudget sets the token budget for the engine. When set,
// agents check remaining budget before making calls.
func (e *Engine) SetBudget(b *Budget) { e.budget = b }

// Budget returns the engine's current budget (may be nil).
func (e *Engine) Budget() *Budget { return e.budget }

// SetJournal sets the call journal for resume support.
func (e *Engine) SetJournal(j *Journal) { e.journal = j }

// Journal returns the engine's call journal (may be nil).
func (e *Engine) Journal() *Journal { return e.journal }

// Workflows returns a copy of the current workflow list, newest
// first. Safe to call from the UI thread.
func (e *Engine) Workflows() []*Workflow {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]*Workflow, 0, len(e.flows))
	for _, w := range e.flows {
		out = append(out, w)
	}
	// Sort newest first.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].StartedAt.After(out[i].StartedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// Snapshots returns a Snapshot for every workflow the engine
// knows about, newest first. The UI uses this to render the
// Workflows tab. Each snapshot is a deep-enough copy that the
// caller can render without worrying about concurrent
// mutation by the engine goroutines.
func (e *Engine) Snapshots() []Snapshot {
	wfs := e.Workflows()
	out := make([]Snapshot, 0, len(wfs))
	for _, w := range wfs {
		out = append(out, e.Snapshot(w.ID))
	}
	return out
}

// Get returns a workflow by id, or nil.
func (e *Engine) Get(id string) *Workflow {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.flows[id]
}

// Snapshot returns a point-in-time copy of the workflow's state.
// Safe to call from the UI thread.
func (e *Engine) Snapshot(id string) Snapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	w := e.flows[id]
	if w == nil {
		return Snapshot{}
	}
	steps := make([]Step, 0, len(w.Steps))
	done := 0
	for _, s := range w.Steps {
		steps = append(steps, *s)
		if s.Status == StepDone {
			done++
		}
	}
	// TotalSteps = current steps + the final synthesis step
	// (the engine always adds an implicit "synthesise" step
	// at the end, which the UI should count as part of the
	// progress meter).
	total := len(w.Steps)
	if total == 0 {
		// planning phase: nothing to count yet
	} else {
		// +1 for the synthesis step
		total = total + 1
	}
	currentMsg, _ := w.currentMsg.Load().(string)
	var budgetSpent, budgetTotal int64
	if w.Budget != nil {
		budgetSpent = w.Budget.Spent()
		budgetTotal = w.Budget.Total()
	}
	return Snapshot{
		ID:          w.ID,
		Name:        w.Name,
		Goal:        w.Goal,
		Status:      w.Status,
		StartedAt:   w.StartedAt,
		EndedAt:     w.EndedAt,
		Duration:    time.Since(w.StartedAt),
		Steps:       steps,
		Summary:     w.Summary,
		CurrentMsg:  currentMsg,
		DoneSteps:   done,
		TotalSteps:  total,
		BudgetSpent: budgetSpent,
		BudgetTotal: budgetTotal,
	}
}

// SetStatus updates the workflow's status string.
func (w *Workflow) setStatus(s string) {
	w.Status = s
	if s == StepDone || s == StepFailed || s == StepCancelled {
		w.EndedAt = time.Now()
	}
}

// SetCurrentMsg updates the "currently doing X" message for the
// active step. The UI polls this to render a live progress line.
func (w *Workflow) setCurrentMsg(s string) {
	w.currentMsg.Store(s)
}

// Start launches a new workflow and returns its id. The workflow
// runs in a goroutine; the caller can poll via Snapshot() to see
// progress, or stop it with Cancel(id).
//
// The `name` is what the user sees in the Workflows tab; `goal`
// is the actual prompt sent to the planner.
func (e *Engine) Start(ctx context.Context, name, goal, strategy string, maxAgents int) (string, error) {
	if name == "" {
		name = "workflow-" + time.Now().Format("150405")
	}
	if strategy == "" {
		strategy = "development"
	}
	if maxAgents <= 0 {
		maxAgents = 5
	}
	id := "wf-" + time.Now().Format("20060102-150405.000000")
	wf := &Workflow{
		ID:        id,
		Name:      name,
		Goal:      goal,
		Strategy:  strategy,
		MaxAgents: maxAgents,
		Status:    "planning",
		StartedAt: time.Now(),
		Budget:    &Budget{}, // per-run budget
		Journal:   e.journal, // inherit engine journal for resume
	}
	// Inherit engine budget total if set
	if e.budget != nil && e.budget.Total() > 0 {
		wf.Budget.SetTotal(e.budget.Total())
	}
	rctx, cancel := context.WithCancel(ctx)
	wf.cancel = cancel
	e.mu.Lock()
	e.flows[id] = wf
	e.cancel[id] = cancel
	e.mu.Unlock()

	if e.hooks.OnWorkflowStart != nil {
		e.hooks.OnWorkflowStart(e.Snapshot(id))
	}
	go e.run(rctx, wf)
	return id, nil
}

// Cancel stops a running workflow and all of its agents. The
// workflow's status becomes "cancelled" and the UI's live
// progress panel is updated.
func (e *Engine) Cancel(id string) {
	e.mu.RLock()
	cancel, ok := e.cancel[id]
	wf := e.flows[id]
	e.mu.RUnlock()
	if !ok || wf == nil {
		return
	}
	cancel()
	wf.setStatus(StepCancelled)
	for _, s := range wf.Steps {
		if s.Status == StepPending || s.Status == StepInProgress {
			s.Status = StepCancelled
		}
	}
	if e.hooks.OnWorkflowComplete != nil {
		e.hooks.OnWorkflowComplete(e.Snapshot(id))
	}
}

// CancelStep stops a single step's agent without cancelling the
// rest of the workflow. The step is marked as "cancelled" and
// the workflow continues with the next step.
//
// (Useful when one agent is stuck and the user wants to
// proceed without waiting for it.)
func (e *Engine) CancelStep(workflowID, stepID string) {
	e.mu.RLock()
	wf := e.flows[workflowID]
	e.mu.RUnlock()
	if wf == nil {
		return
	}
	for _, s := range wf.Steps {
		if s.ID == stepID && (s.Status == StepPending || s.Status == StepInProgress) {
			s.Status = StepCancelled
			s.EndedAt = time.Now()
			s.Duration = s.EndedAt.Sub(s.StartedAt)
			if e.hooks.OnStepDone != nil {
				e.hooks.OnStepDone(workflowID, stepID, e.Snapshot(workflowID))
			}
			return
		}
	}
}

// run is the main workflow loop. It plans, executes each step,
// then synthesises a final summary.
func (e *Engine) run(ctx context.Context, wf *Workflow) {
	defer func() {
		if r := recover(); r != nil {
			wf.setStatus(StepFailed)
			wf.Summary = fmt.Sprintf("workflow panic: %v", r)
		}
	}()

	// Phase 1: plan. The planner role produces a list of
	// steps. We do this in-process (no sub-agent spawn) so
	// the user gets the plan immediately.
	plannerRole := findRole("planner")
	if plannerRole == nil {
		wf.setStatus(StepFailed)
		return
	}
	wf.setStatus("running")
	plan, err := e.callRole(ctx, *plannerRole, buildPlannerPrompt(wf), wf)
	if err != nil {
		wf.setStatus(StepFailed)
		wf.Summary = "planner failed: " + err.Error()
		if e.hooks.OnWorkflowComplete != nil {
			e.hooks.OnWorkflowComplete(e.Snapshot(wf.ID))
		}
		return
	}
	steps := parsePlan(plan)
	if len(steps) == 0 {
		// No plan came back \u2014 fall back to a single
		// developer task so the user still gets
		// something.
		steps = []*Step{{
			ID:          "step-1",
			Description: wf.Goal,
			Role:        "developer",
			Status:      StepPending,
		}}
	}
	wf.Steps = steps

	// Phase 2: execute. Sequential for now (parallel is a
	// follow-up; sequential is easier to reason about and
	// matches the existing swarm.Orchestrator behaviour).
	for _, step := range wf.Steps {
		select {
		case <-ctx.Done():
			step.Status = StepCancelled
			continue
		default:
		}
		role := findRole(step.Role)
		if role == nil {
			role = findRole("developer")
		}
		step.Role = role.Name
		step.Status = StepInProgress
		step.StartedAt = time.Now()
		wf.setCurrentMsg(fmt.Sprintf("%s: %s", role.Name, step.Description))
		if e.hooks.OnStepStart != nil {
			e.hooks.OnStepStart(wf.ID, step.ID, e.Snapshot(wf.ID))
		}
		out, err := e.callRole(ctx, *role, buildStepPrompt(wf, step), wf)
		step.EndedAt = time.Now()
		step.Duration = step.EndedAt.Sub(step.StartedAt)
		if err != nil {
			if ctx.Err() != nil {
				step.Status = StepCancelled
			} else {
				step.Status = StepFailed
				step.Output = "error: " + err.Error()
			}
		} else {
			step.Output = out
			if step.Status != StepCancelled {
				step.Status = StepDone
			}
		}
		wf.setCurrentMsg("")
		if e.hooks.OnStepDone != nil {
			e.hooks.OnStepDone(wf.ID, step.ID, e.Snapshot(wf.ID))
		}
	}

	// Phase 3: synthesise. The planner role takes all
	// step outputs and produces a final summary.
	wf.setStatus("synthesizing")
	wf.setCurrentMsg("synthesising final report")
	if e.hooks.OnStepStart != nil {
		e.hooks.OnStepStart(wf.ID, "__synth__", e.Snapshot(wf.ID))
	}
	final, err := e.callRole(ctx, *plannerRole, buildSynthesisPrompt(wf), wf)
	wf.setCurrentMsg("")
	if err != nil || final == "" {
		final = buildFallbackSummary(wf)
	}
	wf.Summary = final
	allOK := true
	for _, s := range wf.Steps {
		if s.Status != StepDone {
			allOK = false
			break
		}
	}
	if allOK {
		wf.setStatus(StepDone)
	} else {
		wf.setStatus(StepFailed)
	}
	if e.hooks.OnStepDone != nil {
		e.hooks.OnStepDone(wf.ID, "__synth__", e.Snapshot(wf.ID))
	}
	if e.hooks.OnWorkflowComplete != nil {
		e.hooks.OnWorkflowComplete(e.Snapshot(wf.ID))
	}
}

// callRole invokes the LLM with a role's system prompt + the
// given user prompt. Uses the engine's config to resolve the
// active model and API key.
//
// Budget and journal integration:
//   - Checks wf.Budget.Exhausted() before calling; returns an
//     error if the budget cap has been reached.
//   - Estimates token usage from response length and calls
//     wf.Budget.Spend(n) after a successful call.
//   - Records a JournalEntry if wf.Journal is non-nil for
//     deterministic resume on re-run.
func (e *Engine) callRole(ctx context.Context, role Role, prompt string, wf *Workflow) (string, error) {
	// Journal replay: if the workflow has a journal and a
	// matching entry exists (same prompt hash), return the
	// cached result instead of making a new LLM call. This
	// enables deterministic resume of interrupted runs.
	if wf.Journal != nil {
		cached := wf.Journal.Replay([]string{prompt})
		if len(cached) > 0 && cached[0].Output != "" {
			return cached[0].Output, nil
		}
	}

	// Budget gate
	if wf.Budget != nil && wf.Budget.Exhausted() {
		return "", fmt.Errorf("budget exhausted: spent %d of %d tokens",
			wf.Budget.Spent(), wf.Budget.Total())
	}

	// Resolve model config. If the engine has no config (e.g.
	// headless mode without a cortex config file), fall back to
	// env vars per provider.
	if e.cfg == nil {
		return "", fmt.Errorf("workflow engine has no config — set CORTEX_API_KEY or provider-specific env vars")
	}
	_, mc, err := e.cfg.GetModel(e.cfg.DefaultModel)
	if err != nil {
		return "", err
	}
	apiKey := mc.APIKey
	if apiKey == "" {
		// Fall back to env-var-driven providers.
		if env := cortexconfig.ProviderEnvVar(mc.Provider); env != "" {
			apiKey = getenv(env)
		}
	}
	if apiKey == "" {
		return "", fmt.Errorf("no API key for provider %q", mc.Provider)
	}
	prov, err := llmprovider.New(llmprovider.ModelConfig{
		Provider: mc.Provider,
		Model:    mc.Model,
		BaseURL:  mc.BaseURL,
		APIKey:   apiKey,
	})
	if err != nil {
		return "", err
	}
	req := llmprovider.Request{
		Model: mc.Model,
		Messages: []llmprovider.Message{
			{Role: "system", Content: role.SystemPrompt},
			{Role: "user", Content: prompt},
		},
	}
	resp, err := prov.Chat(ctx, req)
	if err != nil {
		return "", err
	}

	// Budget: estimate tokens used (~ chars/4 for output, add
	// prompt size estimate for input). This is a rough heuristic
	// since we don't always get usage back from all providers.
	estimatedTokens := int64(len(prompt)/4 + len(resp.Content)/4)
	if wf.Budget != nil {
		wf.Budget.Spend(estimatedTokens)
	}

	// Journal: record this call for deterministic resume
	if wf.Journal != nil {
		wf.Journal.Record(JournalEntry{
			Prompt: prompt,
			Output: resp.Content,
		})
	}

	return resp.Content, nil
}

// findRole returns a pointer to the role with the given name, or
// nil. Used so callers can update the role struct in place.
func findRole(name string) *Role {
	for i := range BuiltinRoles {
		if BuiltinRoles[i].Name == name {
			return &BuiltinRoles[i]
		}
	}
	return nil
}

// buildPlannerPrompt returns the prompt sent to the planner role
// at workflow start. The planner must output a JSON plan.
func buildPlannerPrompt(wf *Workflow) string {
	roles := make([]string, 0, len(wf.Steps))
	seen := map[string]bool{}
	for _, r := range BuiltinRoles {
		seen[r.Name] = true
		roles = append(roles, r.Name)
	}
	return fmt.Sprintf(`You are planning a multi-agent workflow.

Goal: %s
Strategy: %s
Max agents: %d
Available roles: %s

Break the goal into 3-7 concrete steps. Each step should be assignable to one role and complete in a single LLM call. Output ONLY a JSON object in this exact schema (no markdown, no prose, no preamble):

{"steps": [{"id":"step-1","description":"...","role":"developer"}, {"id":"step-2","description":"...","role":"reviewer"}]}

Use the roles in this order when sensible: planner (already done), researcher, developer, reviewer, tester, fixer, documenter. Skip roles that don't apply.

Keep step descriptions short (under 100 chars). The orchestrator will run them in order.`, wf.Goal, wf.Strategy, wf.MaxAgents, strings.Join(roles, ", "))
}

// buildStepPrompt returns the prompt sent to the agent for a
// given step. It includes the workflow goal, the step's
// description, and the outputs of any previous steps so the
// agent has the full context.
func buildStepPrompt(wf *Workflow, step *Step) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Workflow goal: %s\n", wf.Goal)
	fmt.Fprintf(&b, "Your role: %s\n", step.Role)
	fmt.Fprintf(&b, "Your task: %s\n\n", step.Description)
	if len(wf.Steps) > 1 {
		b.WriteString("Previous steps in this workflow:\n")
		for _, s := range wf.Steps {
			if s.ID == step.ID {
				break
			}
			if s.Output == "" {
				continue
			}
			fmt.Fprintf(&b, "--- %s (%s) ---\n%s\n\n", s.ID, s.Role, s.Output)
		}
	}
	b.WriteString("Complete the task. Be concise. Output your result directly. No preamble.")
	return b.String()
}

// buildSynthesisPrompt is the final prompt sent to the planner
// to roll up all step outputs into a single summary.
func buildSynthesisPrompt(wf *Workflow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Workflow goal: %s\n\n", wf.Goal)
	b.WriteString("Step outputs:\n\n")
	for _, s := range wf.Steps {
		fmt.Fprintf(&b, "## %s [%s] (%s)\n", s.ID, s.Role, s.Status)
		if s.Output != "" {
			b.WriteString(s.Output)
		} else {
			b.WriteString("(no output)")
		}
		b.WriteString("\n\n")
	}
	b.WriteString("Synthesise a final, coherent report. Highlight the key findings, decisions, and any next steps. Be concise.")
	return b.String()
}

// buildFallbackSummary is used when the synthesis call fails.
// It concatenates the step outputs with a header so the user
// still gets something useful.
func buildFallbackSummary(wf *Workflow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", wf.Name)
	fmt.Fprintf(&b, "_Synthesis failed \u2014 showing raw step outputs._\n\n")
	for _, s := range wf.Steps {
		fmt.Fprintf(&b, "## %s [%s]\n", s.ID, s.Role)
		if s.Output != "" {
			b.WriteString(s.Output)
		} else {
			b.WriteString("(no output)")
		}
		b.WriteString("\n\n")
	}
	return b.String()
}

// parsePlan extracts a JSON plan from the planner's output.
// Tries fenced ```json first, then a balanced top-level { }.
func parsePlan(content string) []*Step {
	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return nil
	}
	var parsed struct {
		Steps []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			Role        string `json:"role"`
		} `json:"steps"`
	}
	if err := jsonUnmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil
	}
	out := make([]*Step, 0, len(parsed.Steps))
	for _, t := range parsed.Steps {
		if t.ID == "" {
			t.ID = fmt.Sprintf("step-%d", len(out)+1)
		}
		out = append(out, &Step{
			ID:          t.ID,
			Description: t.Description,
			Role:        t.Role,
			Status:      StepPending,
		})
	}
	return out
}

// getenv wraps os.Getenv so the engine doesn't have to import
// "os" directly (and to make the tests easier to override).
func getenv(name string) string {
	return osGetenv(name)
}
