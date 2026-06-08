package memory

import (
	"os"
	"sort"
	"strings"
)

// RegenerateContext rebuilds .cortex/context.md from high-importance memories.
// The output stays compact — never a full memory dump.
func RegenerateContext(s *Store) error {
	if s == nil {
		return nil
	}
	entries, err := s.List()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Importance == entries[j].Importance {
			return entries[i].UpdatedAt.After(entries[j].UpdatedAt)
		}
		return entries[i].Importance > entries[j].Importance
	})
	maxLines := 80
	if maxLines > len(entries)*2 {
		maxLines = min(80, len(entries)*3)
	}
	sections := map[Type][]string{}
	for _, e := range entries {
		if e.Importance < 0.6 {
			continue
		}
		sections[e.Type] = appendUnique(sections[e.Type], e.Content)
	}
	var b strings.Builder
	b.WriteString("# Project Context\n\n")
	b.WriteString("_Auto-generated summary of durable project memories. Edit sparingly._\n\n")
	typeOrder := []Type{TypeArchitecture, TypeConvention, TypePreference, TypeWorkflow, TypeProjectFact}
	lines := 4
	for _, typ := range typeOrder {
		items := sections[typ]
		if len(items) == 0 {
			continue
		}
		b.WriteString("## " + titleForType(typ) + "\n")
		lines++
		for _, item := range items {
			if lines >= maxLines {
				break
			}
			b.WriteString("- " + item + "\n")
			lines++
		}
		b.WriteString("\n")
		lines++
	}
	text := strings.TrimSpace(b.String())
	if len(text) > s.limits.ContextMaxBytes {
		text = text[:s.limits.ContextMaxBytes]
		if i := strings.LastIndex(text, "\n"); i > 0 {
			text = text[:i]
		}
	}
	path := s.paths.ContextMD()
	if err := os.MkdirAll(s.project, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text+"\n"), 0o644)
}

func titleForType(typ Type) string {
	switch typ {
	case TypeArchitecture:
		return "Architecture"
	case TypeConvention:
		return "Coding conventions"
	case TypePreference:
		return "Preferences"
	case TypeWorkflow:
		return "Workflow"
	case TypeProjectFact:
		return "Project facts"
	default:
		return string(typ)
	}
}

func appendUnique(items []string, v string) []string {
	v = strings.TrimSpace(v)
	for _, existing := range items {
		if strings.EqualFold(existing, v) {
			return items
		}
	}
	return append(items, v)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}