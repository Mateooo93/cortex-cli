package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// persistedWorkflow is the on-disk representation of a completed workflow.
type persistedWorkflow struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Goal          string              `json:"goal"`
	Status        string              `json:"status"`
	StartedAt     time.Time           `json:"startedAt"`
	EndedAt       time.Time           `json:"endedAt"`
	Duration      string              `json:"duration"`
	Steps         []persistedStep     `json:"steps"`
	Summary       string              `json:"summary"`
	Preset        string              `json:"preset"`
	BudgetSpent   int64               `json:"budgetSpent"`
	BudgetTotal   int64               `json:"budgetTotal"`
}

type persistedStep struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	Output      string `json:"output"`
	Duration    string `json:"duration"`
}

// SaveWorkflow persists a completed workflow snapshot to disk.
// Workflows are stored in ~/.cortex/workflows/<id>.json.
func SaveWorkflow(cfg *cortexconfig.Config, snap Snapshot) error {
	if snap.ID == "" {
		return nil
	}
	dir := workflowsDir(cfg)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	pw := persistedWorkflow{
		ID:          snap.ID,
		Name:        snap.Name,
		Goal:        snap.Goal,
		Status:      snap.Status,
		StartedAt:   snap.StartedAt,
		EndedAt:     snap.EndedAt,
		Duration:    snap.Duration.String(),
		Summary:     snap.Summary,
		BudgetSpent: snap.BudgetSpent,
		BudgetTotal: snap.BudgetTotal,
	}
	for _, s := range snap.Steps {
		pw.Steps = append(pw.Steps, persistedStep{
			ID:          s.ID,
			Description: s.Description,
			Role:        s.Role,
			Status:      s.Status,
			Output:      s.Output,
			Duration:    s.Duration.String(),
		})
	}

	data, err := json.MarshalIndent(pw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, snap.ID+".json"), data, 0644)
}

// LoadWorkflows reads all persisted workflows from disk.
// Returns a slice of snapshots, newest first.
func LoadWorkflows(cfg *cortexconfig.Config) ([]Snapshot, error) {
	dir := workflowsDir(cfg)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var snaps []Snapshot
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var pw persistedWorkflow
		if err := json.Unmarshal(data, &pw); err != nil {
			continue
		}
		dur, _ := time.ParseDuration(pw.Duration)
		snap := Snapshot{
			ID:          pw.ID,
			Name:        pw.Name,
			Goal:        pw.Goal,
			Status:      pw.Status,
			StartedAt:   pw.StartedAt,
			EndedAt:     pw.EndedAt,
			Duration:    dur,
			Summary:     pw.Summary,
			BudgetSpent: pw.BudgetSpent,
			BudgetTotal: pw.BudgetTotal,
		}
		for _, s := range pw.Steps {
			sd, _ := time.ParseDuration(s.Duration)
			snap.Steps = append(snap.Steps, Step{
				ID:          s.ID,
				Description: s.Description,
				Role:        s.Role,
				Status:      s.Status,
				Output:      s.Output,
				Duration:    sd,
			})
		}
		snaps = append(snaps, snap)
	}

	// Sort newest first
	for i := 0; i < len(snaps); i++ {
		for j := i + 1; j < len(snaps); j++ {
			if snaps[j].StartedAt.After(snaps[i].StartedAt) {
				snaps[i], snaps[j] = snaps[j], snaps[i]
			}
		}
	}

	return snaps, nil
}

// AutoSaveCompleted wraps the existing OnWorkflowComplete hook to also
// persist completed workflows to disk. It preserves all existing hooks.
func (e *Engine) AutoSaveCompleted(cfg *cortexconfig.Config) {
	prevComplete := e.hooks.OnWorkflowComplete
	e.hooks.OnWorkflowComplete = func(snap Snapshot) {
		if prevComplete != nil {
			prevComplete(snap)
		}
		if snap.Status == "done" || snap.Status == "failed" || snap.Status == "cancelled" {
			_ = SaveWorkflow(cfg, snap)
		}
	}
}

// workflowsDir returns the path to the workflows storage directory.
func workflowsDir(cfg *cortexconfig.Config) string {
	if cfg != nil {
		return filepath.Join(cortexconfig.Dir(), "workflows")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cortex", "workflows")
}
