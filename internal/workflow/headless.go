// Package workflow — headless CLI for cortex workflows.
//
// Provides RunHeadless, ShowStatus, and ExportResult for
// headless (non-TUI) workflow execution. Intended for CI,
// scripts, and oh-my-hermes worker integrations.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// HeadlessConfig is the input for RunHeadless.
type HeadlessConfig struct {
	Preset    string        // workflow preset name (default "code")
	Goal      string        // the goal/task (required)
	MaxAgents int           // max agent count (default 5)
	Timeout   time.Duration // max runtime (default 10m)
	Budget    int64         // token budget cap (0 = unlimited)
	JSON      bool          // output JSON instead of text
}

// HeadlessResult is returned by RunHeadless on completion.
type HeadlessResult struct {
	Snapshot Snapshot
	JSON     string // pre-serialized JSON if JSON=true
}

// RunHeadless starts a workflow and blocks until completion or timeout.
// Returns the final snapshot and, if JSON output was requested,
// pre-serialized JSON suitable for stdout.
func RunHeadless(cfg HeadlessConfig) (*HeadlessResult, error) {
	if cfg.Goal == "" {
		return nil, fmt.Errorf("Goal is required")
	}
	if cfg.Preset == "" {
		cfg.Preset = "code"
	}
	if cfg.MaxAgents <= 0 {
		cfg.MaxAgents = 5
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Minute
	}

	// Resolve preset
	var preset *Preset
	for i := range BuiltinPresets {
		if BuiltinPresets[i].Name == cfg.Preset {
			preset = &BuiltinPresets[i]
			break
		}
	}
	if preset == nil {
		return nil, fmt.Errorf("unknown preset: %s (valid: code, research, test, review, docs)", cfg.Preset)
	}
	mc := *preset
	mc.MaxAgents = cfg.MaxAgents

	// Load the real cortex config so the engine can resolve
	// providers and API keys. Falls back to env vars if the
	// config cannot be loaded.
	cortexCfg := loadCortexConfig()
	engine := NewEngine(cortexCfg)
	if cfg.Budget > 0 {
		b := &Budget{}
		b.SetTotal(cfg.Budget)
		engine.SetBudget(b)
	}

	journal := NewJournal("")
	engine.SetJournal(journal)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	fmt.Fprintf(os.Stderr, "Starting workflow: %s\nGoal: %s\n", mc.Name, cfg.Goal)

	id, err := engine.Start(ctx, mc.Name, cfg.Goal, mc.Strategy, mc.MaxAgents)
	if err != nil {
		return nil, fmt.Errorf("failed to start workflow: %w", err)
	}

	// Poll until done
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("workflow timed out after %v", cfg.Timeout)
		case <-ticker.C:
			snap := engine.Snapshot(id)
			if snap.Status == "done" || snap.Status == "failed" || snap.Status == "cancelled" {
				result := &HeadlessResult{Snapshot: snap}
				if cfg.JSON {
					result.JSON = marshalSnapshotJSON(snap)
				}
				return result, nil
			}
		}
	}
}

// ShowStatus prints the current state of all workflows to stdout.
func ShowStatus(engine *Engine) {
	snapshots := engine.Snapshots()
	if len(snapshots) == 0 {
		fmt.Println("No workflows have been run in this session.")
		return
	}
	for _, snap := range snapshots {
		elapsed := time.Since(snap.StartedAt).Truncate(time.Second)
		fmt.Printf("%s  %s  %s  %d/%d steps  %s",
			snap.ID, snap.Name, snap.Status,
			snap.DoneSteps, snap.TotalSteps, elapsed)
		if snap.BudgetTotal > 0 {
			fmt.Printf("  budget: %d/%d", snap.BudgetSpent, snap.BudgetTotal)
		}
		fmt.Println()
	}
}

// ExportResult returns the JSON representation of a workflow snapshot.
func ExportResult(engine *Engine, id string) (string, error) {
	snap := engine.Snapshot(id)
	if snap.ID == "" {
		return "", fmt.Errorf("workflow %q not found", id)
	}
	return marshalSnapshotJSON(snap), nil
}

// PrintResult writes a human-readable workflow result to stdout.
func PrintResult(snap Snapshot) {
	fmt.Printf("# %s (%s)\n", snap.Name, snap.Status)
	fmt.Printf("ID: %s\n", snap.ID)
	fmt.Printf("Goal: %s\n", snap.Goal)
	fmt.Printf("Steps: %d/%d complete\n", snap.DoneSteps, snap.TotalSteps)
	fmt.Printf("Duration: %s\n", snap.Duration.Truncate(time.Second))
	if snap.BudgetTotal > 0 {
		fmt.Printf("Budget: %d/%d tokens\n", snap.BudgetSpent, snap.BudgetTotal)
	}
	fmt.Println()

	for _, step := range snap.Steps {
		marker := " "
		switch step.Status {
		case StepDone:
			marker = "✓"
		case StepFailed:
			marker = "✗"
		case StepInProgress:
			marker = "●"
		}
		fmt.Printf("  %s %s [%s] (%s)\n", marker, step.Description, step.Role, step.Status)
		if step.Output != "" {
			for _, line := range strings.Split(step.Output, "\n") {
				fmt.Printf("    %s\n", line)
			}
		}
	}

	if snap.Summary != "" {
		fmt.Printf("\n--- Summary ---\n%s\n", snap.Summary)
	}
}

// marshalSnapshotJSON serializes a snapshot as indented JSON.
func marshalSnapshotJSON(snap Snapshot) string {
	type stepOut struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Role        string `json:"role"`
		Status      string `json:"status"`
		Output      string `json:"output"`
	}
	type wfOut struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		Goal        string    `json:"goal"`
		Status      string    `json:"status"`
		StartedAt   time.Time `json:"startedAt"`
		Duration    string    `json:"duration"`
		DoneSteps   int       `json:"doneSteps"`
		TotalSteps  int       `json:"totalSteps"`
		Summary     string    `json:"summary"`
		BudgetSpent int64     `json:"budgetSpent"`
		BudgetTotal int64     `json:"budgetTotal"`
		Steps       []stepOut `json:"steps"`
	}
	out := wfOut{
		ID:          snap.ID,
		Name:        snap.Name,
		Goal:        snap.Goal,
		Status:      snap.Status,
		StartedAt:   snap.StartedAt,
		Duration:    snap.Duration.String(),
		DoneSteps:   snap.DoneSteps,
		TotalSteps:  snap.TotalSteps,
		Summary:     snap.Summary,
		BudgetSpent: snap.BudgetSpent,
		BudgetTotal: snap.BudgetTotal,
		Steps:       make([]stepOut, 0, len(snap.Steps)),
	}
	for _, s := range snap.Steps {
		out.Steps = append(out.Steps, stepOut{
			ID:          s.ID,
			Description: s.Description,
			Role:        s.Role,
			Status:      s.Status,
			Output:      s.Output,
		})
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	return string(data)
}

// loadCortexConfig attempts to load the user's cortex config.
// Falls back to Default() if the config file cannot be read
// (e.g. in CI environments without a ~/.cortex directory).
func loadCortexConfig() *cortexconfig.Config {
	cfg, err := cortexconfig.Load()
	if err != nil || cfg == nil {
		return cortexconfig.Default()
	}
	return cfg
}
