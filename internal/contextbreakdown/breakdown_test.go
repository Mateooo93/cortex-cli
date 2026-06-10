package contextbreakdown

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
)

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Fatalf("empty = %d, want 0", got)
	}
	if got := EstimateTokens("abcd"); got != 1 {
		t.Fatalf("short = %d, want 1", got)
	}
	if got := EstimateTokens(string(make([]byte, 400))); got != 100 {
		t.Fatalf("400 bytes = %d, want 100", got)
	}
}

func TestCompute_IncludesSystemAndConversation(t *testing.T) {
	dir := t.TempDir()
	result := Compute(Input{
		Workdir:    dir,
		UsedTokens: 5000,
		MaxTokens:  128000,
		History: []HistoryMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
	})
	if result.Used != 5000 {
		t.Fatalf("used = %d, want 5000", result.Used)
	}
	if result.Max != 128000 {
		t.Fatalf("max = %d, want 128000", result.Max)
	}
	foundSystem := false
	foundConversation := false
	for _, item := range result.Items {
		if item.ID == "system" {
			foundSystem = true
			if item.Tokens <= 0 {
				t.Fatal("system tokens should be positive")
			}
		}
		if item.ID == "conversation" {
			foundConversation = true
		}
	}
	if !foundSystem {
		t.Fatal("missing system item")
	}
	if !foundConversation {
		t.Fatal("missing conversation item")
	}
}

func TestCompute_ScansSkillsAndAgentsMD(t *testing.T) {
	dir := t.TempDir()
	cortexDir := filepath.Join(dir, ".cortex")
	skillsDir := filepath.Join(cortexDir, "skills", "demo-skill")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("skill content here"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("agent guidance"), 0o644); err != nil {
		t.Fatal(err)
	}

	paths := config.NewCortexPaths("", "", dir)
	result := Compute(Input{
		Workdir: dir,
		Paths:   paths,
		History: []HistoryMessage{{Role: "user", Content: "test"}},
	})

	var skills, agentsMD *Item
	for i := range result.Items {
		switch result.Items[i].ID {
		case "skills":
			skills = &result.Items[i]
		case "agents-md":
			agentsMD = &result.Items[i]
		}
	}
	if skills == nil || len(skills.Children) != 1 {
		t.Fatalf("skills item: %+v", skills)
	}
	if agentsMD == nil || agentsMD.Tokens <= 0 {
		t.Fatalf("agents-md item: %+v", agentsMD)
	}
}
