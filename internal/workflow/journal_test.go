package workflow

import "testing"

func TestJournalReplayMatchesPrefixOnly(t *testing.T) {
	j := NewJournal("")
	j.Record(JournalEntry{Prompt: "planner prompt", Output: "plan"})
	j.Record(JournalEntry{Prompt: "developer prompt", Output: "implementation"})

	matches := j.Replay([]string{"planner prompt", "developer prompt"})
	if len(matches) != 2 {
		t.Fatalf("Replay returned %d matches, want 2", len(matches))
	}
	if matches[0].Output != "plan" || matches[1].Output != "implementation" {
		t.Fatalf("Replay returned wrong outputs: %+v", matches)
	}

	matches = j.Replay([]string{"developer prompt"})
	if len(matches) != 0 {
		t.Fatalf("Replay should not match non-prefix prompts, got %+v", matches)
	}
}

func TestJournalFindByPromptMatchesAnyEntry(t *testing.T) {
	j := NewJournal("")
	j.Record(JournalEntry{Prompt: "planner prompt", Output: "plan"})
	j.Record(JournalEntry{Prompt: "developer prompt", Output: "implementation"})

	entry, ok := j.FindByPrompt("developer prompt")
	if !ok {
		t.Fatal("FindByPrompt did not find non-prefix entry")
	}
	if entry.Output != "implementation" {
		t.Fatalf("FindByPrompt output = %q, want %q", entry.Output, "implementation")
	}

	if _, ok := j.FindByPrompt("missing prompt"); ok {
		t.Fatal("FindByPrompt matched missing prompt")
	}
}
