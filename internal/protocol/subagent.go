package protocol

// LocalSubagentStatus is the lifecycle state of a local sub-agent.
type LocalSubagentStatus string

const (
	LocalSubagentRunning LocalSubagentStatus = "running"
	LocalSubagentDone    LocalSubagentStatus = "done"
	LocalSubagentFailed  LocalSubagentStatus = "failed"
)

// LocalSubagentItem describes one local sub-agent shown in the right panel.
type LocalSubagentItem struct {
	ID        string              `json:"id"`
	Role      string              `json:"role"`
	Task      string              `json:"task"`
	Status    LocalSubagentStatus `json:"status"`
	Output    string              `json:"output,omitempty"`
	Error     string              `json:"error,omitempty"`
	StartedAt int64               `json:"started_at_unix"`
	EndedAt   int64               `json:"ended_at_unix,omitempty"`
}

// EventLocalSubagentsUpdated carries the session's local sub-agents.
type EventLocalSubagentsUpdated struct {
	Subagents []LocalSubagentItem `json:"subagents"`
}