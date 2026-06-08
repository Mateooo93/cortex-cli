package memory

import (
	"fmt"
	"sort"
	"strings"
)

// RetrieveRelevant returns top memories for prompt injection, ranked by
// importance and optional query relevance. The Retriever interface allows
// swapping in embedding backends later without changing callers.
type Retriever interface {
	RetrieveRelevant(query string, limit int, maxBytes int) ([]Entry, error)
}

// RetrieveRelevant implements keyword relevance + importance ranking.
func (s *Store) RetrieveRelevant(query string, limit int, maxBytes int) ([]Entry, error) {
	if s == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = s.limits.MaxInjectEntries
	}
	if maxBytes <= 0 {
		maxBytes = s.limits.MaxInjectBytes
	}
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	type scored struct {
		entry Entry
		score float64
	}
	scoredEntries := make([]scored, 0, len(all))
	for _, e := range all {
		score := e.Importance
		if query != "" {
			lower := strings.ToLower(e.Content + " " + string(e.Type))
			if strings.Contains(lower, query) {
				score += 0.35
			} else {
				// Skip low-relevance rows when the user/model gave a query hint.
				words := strings.Fields(query)
				matched := 0
				for _, w := range words {
					if len(w) < 3 {
						continue
					}
					if strings.Contains(lower, w) {
						matched++
					}
				}
				if matched == 0 {
					continue
				}
				score += 0.15 * float64(matched)
			}
		}
		scoredEntries = append(scoredEntries, scored{entry: e, score: score})
	}
	sort.Slice(scoredEntries, func(i, j int) bool {
		if scoredEntries[i].score == scoredEntries[j].score {
			return scoredEntries[i].entry.UpdatedAt.After(scoredEntries[j].entry.UpdatedAt)
		}
		return scoredEntries[i].score > scoredEntries[j].score
	})
	var out []Entry
	used := 0
	for _, se := range scoredEntries {
		if len(out) >= limit {
			break
		}
		line := se.entry.Content
		if used+len(line) > maxBytes {
			break
		}
		out = append(out, se.entry)
		used += len(line)
	}
	return out, nil
}

// FormatInjection builds the memory block appended to the system prompt.
func (s *Store) FormatInjection(query string) string {
	if s == nil {
		return ""
	}
	var parts []string
	if ctx := s.ReadContextMD(); ctx != "" {
		parts = append(parts, "## Project context\n"+ctx)
	}
	entries, err := s.RetrieveRelevant(query, s.limits.MaxInjectEntries, s.limits.MaxInjectBytes)
	if err != nil || len(entries) == 0 {
		if len(parts) == 0 {
			return ""
		}
		return strings.Join(parts, "\n\n")
	}
	var lines []string
	for _, e := range entries {
		lines = append(lines, fmt.Sprintf("- [%s] %s", e.Type, e.Content))
	}
	parts = append(parts, "## Relevant project memories\n"+strings.Join(lines, "\n"))
	return strings.Join(parts, "\n\n")
}