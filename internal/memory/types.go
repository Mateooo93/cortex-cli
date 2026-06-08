package memory

import "time"

// Type is a durable memory category. Temporary session facts must not use these.
type Type string

const (
	TypePreference   Type = "preference"
	TypeConvention   Type = "convention"
	TypeArchitecture Type = "architecture"
	TypeWorkflow     Type = "workflow"
	TypeProjectFact  Type = "project_fact"
)

// ValidTypes lists accepted memory categories.
var ValidTypes = []Type{
	TypePreference,
	TypeConvention,
	TypeArchitecture,
	TypeWorkflow,
	TypeProjectFact,
}

// Entry is one persisted project-scoped memory row.
type Entry struct {
	ID         string    `json:"id"`
	Content    string    `json:"content"`
	Type       Type      `json:"type"`
	Importance float64   `json:"importance"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Source     string    `json:"source"`
	Project    string    `json:"project"`
}

// Limits caps store size and prompt injection budget.
type Limits struct {
	MaxEntries       int
	MaxContentLen    int
	MaxInjectEntries int
	MaxInjectBytes   int
	MinImportance    float64
	ContextMaxBytes  int
}

// DefaultLimits returns conservative defaults that keep memory signal-dense.
func DefaultLimits() Limits {
	return Limits{
		MaxEntries:       100,
		MaxContentLen:    500,
		MaxInjectEntries: 8,
		MaxInjectBytes:   2048,
		MinImportance:    0.55,
		ContextMaxBytes:  1024,
	}
}

// Metadata is stored alongside the DB for tooling and future retrieval backends.
type Metadata struct {
	Version       int       `json:"version"`
	Project       string    `json:"project"`
	MemoryCount   int       `json:"memory_count"`
	LastSummarized time.Time `json:"last_summarized,omitempty"`
	Retrieval     string    `json:"retrieval"` // "sqlite_fts" today; embeddings later
}