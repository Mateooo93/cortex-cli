package workflow

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// JournalEntry records one agent call in a workflow run.
// The journal enables deterministic resume: when a run is
// restarted with the same script, completed agent calls
// return their cached results instantly.
type JournalEntry struct {
	Index      int    `json:"index"`      // order in the run
	Prompt     string `json:"prompt"`     // agent prompt
	Label      string `json:"label"`      // display label
	Phase      string `json:"phase"`      // progress phase
	Output     string `json:"output"`     // agent result
	Error      string `json:"error"`      // error message (if any)
	PromptHash string `json:"promptHash"` // sha256 of prompt
}

// Journal is a write-ahead log of agent calls within a run.
// It is safe for concurrent use.
type Journal struct {
	mu       sync.Mutex
	entries  []JournalEntry
	filePath string
}

// NewJournal creates a journal backed by a file on disk.
// If filePath is empty, the journal is in-memory only.
func NewJournal(filePath string) *Journal {
	return &Journal{filePath: filePath}
}

// Record adds an entry to the journal and flushes to disk.
func (j *Journal) Record(entry JournalEntry) {
	entry.PromptHash = hashPrompt(entry.Prompt)
	j.mu.Lock()
	defer j.mu.Unlock()
	entry.Index = len(j.entries)
	j.entries = append(j.entries, entry)
	j.flush()
}

// Entries returns a copy of all recorded entries.
func (j *Journal) Entries() []JournalEntry {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]JournalEntry, len(j.entries))
	copy(out, j.entries)
	return out
}

// Replay compares the current prompt list against the journal.
// It returns the longest prefix of agent calls whose prompts match
// (by hash) the journaled entries. These can be served from cache
// on resume.
//
// Returns nil if there is no saved journal or no match.
func (j *Journal) Replay(prompts []string) []JournalEntry {
	j.mu.Lock()
	defer j.mu.Unlock()

	if len(j.entries) == 0 {
		return nil
	}

	var matches []JournalEntry
	for i, entry := range j.entries {
		if i >= len(prompts) {
			break
		}
		if entry.PromptHash == hashPrompt(prompts[i]) {
			matches = append(matches, entry)
		} else {
			break // prefix must be contiguous
		}
	}
	return matches
}

// FindByPrompt returns the first journal entry with a matching
// prompt hash. Unlike Replay, this is not prefix-based; it is used
// when an individual agent call wants to reuse its cached result.
func (j *Journal) FindByPrompt(prompt string) (JournalEntry, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	hash := hashPrompt(prompt)
	for _, entry := range j.entries {
		if entry.PromptHash == hash {
			return entry, true
		}
	}
	return JournalEntry{}, false
}

// Load reads a journal from disk. Returns nil if the file
// doesn't exist.
func LoadJournal(path string) (*Journal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []JournalEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse journal: %w", err)
	}
	j := &Journal{filePath: path}
	j.entries = entries
	return j, nil
}

// flush writes the journal to disk (caller holds mu).
func (j *Journal) flush() {
	if j.filePath == "" {
		return
	}
	data, err := json.MarshalIndent(j.entries, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(j.filePath)
	os.MkdirAll(dir, 0755)
	os.WriteFile(j.filePath, data, 0644)
}

// hashPrompt returns a short hash of the prompt for matching.
func hashPrompt(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return fmt.Sprintf("%x", h[:8])
}
