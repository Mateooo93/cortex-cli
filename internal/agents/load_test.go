package agents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromDirs_OverrideWins(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home", "agents")
	proj := filepath.Join(dir, "project", ".cortex", "agents")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAgent(t, home, "explore.md", "---\nname: explore\nmodel: anthropic/claude-sonnet-4-6\ntools: read_file\nmax_turns: 5\n---\nExplore only.")
	writeAgent(t, proj, "explore.md", "---\nname: explore\nmodel: openai/gpt-4o\ntools: grep\nmax_turns: 9\n---\nProject explore.")

	catalog, err := LoadFromDirs([]string{home, proj})
	if err != nil {
		t.Fatal(err)
	}
	ag, ok := catalog["explore"]
	if !ok {
		t.Fatal("missing explore agent")
	}
	if ag.Model != "openai/gpt-4o" {
		t.Fatalf("model = %q, want project override", ag.Model)
	}
	if ag.MaxTurns != 9 {
		t.Fatalf("max_turns = %d, want 9", ag.MaxTurns)
	}
}

func TestResolve_RoleAlias(t *testing.T) {
	catalog := map[string]Agent{
		"implementer": {Name: "implementer"},
		"explore":     {Name: "explore"},
	}
	ag, ok := Resolve("developer", catalog)
	if !ok || ag.Name != "implementer" {
		t.Fatalf("Resolve(developer) = %+v, %v", ag, ok)
	}
	ag, ok = Resolve("researcher", catalog)
	if !ok || ag.Name != "explore" {
		t.Fatalf("Resolve(researcher) = %+v, %v", ag, ok)
	}
}

func writeAgent(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}