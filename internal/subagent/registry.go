package subagent

import (
	"fmt"
	"sync"
	"time"
)

// Status is the lifecycle state of a local sub-agent.
type Status string

const (
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

// Subagent is one background local sub-agent dispatched via spawn_agent.
type Subagent struct {
	ID        string
	Role      string
	Task      string
	Status    Status
	Output    string
	Error     string
	StartedAt time.Time
	EndedAt   time.Time
}

// Registry tracks local sub-agents for one session.
type Registry struct {
	mu       sync.RWMutex
	seq      int
	agents   map[string]*Subagent
	onChange func([]Subagent)
}

// NewRegistry constructs an empty registry. onChange is called whenever
// the list changes (may be nil).
func NewRegistry(onChange func([]Subagent)) *Registry {
	return &Registry{
		agents:   make(map[string]*Subagent),
		onChange: onChange,
	}
}

// List returns a snapshot sorted by start time (newest first).
func (r *Registry) List() []Subagent {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Subagent, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, *a)
	}
	// Newest first so the panel shows active work at the top.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].StartedAt.After(out[i].StartedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// RegisterRunning adds a running sub-agent and returns its ID.
func (r *Registry) RegisterRunning(role, task string) string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	r.seq++
	id := fmt.Sprintf("subagent-%d", r.seq)
	r.agents[id] = &Subagent{
		ID:        id,
		Role:      role,
		Task:      task,
		Status:    StatusRunning,
		StartedAt: time.Now(),
	}
	r.mu.Unlock()
	r.notify()
	return id
}

// MarkDone records successful completion.
func (r *Registry) MarkDone(id, output string) {
	r.setFinal(id, StatusDone, output, "")
}

// MarkFailed records failure.
func (r *Registry) MarkFailed(id, errMsg string) {
	r.setFinal(id, StatusFailed, "", errMsg)
}

func (r *Registry) setFinal(id string, status Status, output, errMsg string) {
	if r == nil || id == "" {
		return
	}
	r.mu.Lock()
	a, ok := r.agents[id]
	if ok {
		a.Status = status
		a.Output = output
		a.Error = errMsg
		a.EndedAt = time.Now()
	}
	r.mu.Unlock()
	if ok {
		r.notify()
	}
}

// Get returns a copy of one sub-agent, if present.
func (r *Registry) Get(id string) (Subagent, bool) {
	if r == nil || id == "" {
		return Subagent{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[id]
	if !ok {
		return Subagent{}, false
	}
	return *a, true
}

func (r *Registry) notify() {
	if r.onChange == nil {
		return
	}
	r.onChange(r.List())
}