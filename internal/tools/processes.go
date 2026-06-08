package tools

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

// wrapShellForBackgroundTracking keeps the shell alive until all in-shell
// background jobs finish (e.g. "npm run dev &"). Without this, bash exits
// immediately while the server keeps running and the process panel greys out.
func wrapShellForBackgroundTracking(command string) string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return trimmed
	}
	return "trap 'wait' EXIT; " + trimmed
}

const (
	defaultShellTimeoutSec = 120
	maxShellTimeoutSec     = 600
)

// BackgroundProcess is a shell command the agent started that may still
// be running after run_shell returned (background or detach-on-timeout).
type BackgroundProcess struct {
	ID        string
	PID       int
	Command   string
	CWD       string
	StartedAt time.Time
	Running   bool
	ExitCode  int
}

// ProcessRegistry tracks background shell processes for one session.
type ProcessRegistry struct {
	mu       sync.Mutex
	seq      int
	procs    map[string]*BackgroundProcess
	onChange func([]BackgroundProcess)
}

// NewProcessRegistry constructs an empty registry. onChange is called
// whenever the process list changes (may be nil).
func NewProcessRegistry(onChange func([]BackgroundProcess)) *ProcessRegistry {
	return &ProcessRegistry{
		procs:    make(map[string]*BackgroundProcess),
		onChange: onChange,
	}
}

// List returns a snapshot of all tracked processes.
func (r *ProcessRegistry) List() []BackgroundProcess {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]BackgroundProcess, 0, len(r.procs))
	for _, p := range r.procs {
		out = append(out, *p)
	}
	return out
}

// Register adds a running process and returns its ID.
func (r *ProcessRegistry) Register(pid int, command, cwd string) string {
	if r == nil || pid <= 0 {
		return ""
	}
	r.mu.Lock()
	r.seq++
	id := fmt.Sprintf("proc-%d", r.seq)
	r.procs[id] = &BackgroundProcess{
		ID:        id,
		PID:       pid,
		Command:   command,
		CWD:       cwd,
		StartedAt: time.Now(),
		Running:   true,
		ExitCode:  -1,
	}
	r.mu.Unlock()
	r.notify()
	return id
}

// MarkExited records a process exit.
func (r *ProcessRegistry) MarkExited(id string, exitCode int) {
	if r == nil || id == "" {
		return
	}
	r.mu.Lock()
	p, ok := r.procs[id]
	if ok {
		p.Running = false
		p.ExitCode = exitCode
	}
	r.mu.Unlock()
	if ok {
		r.notify()
	}
}

// Stop sends SIGTERM to a tracked process and marks it stopped.
func (r *ProcessRegistry) Stop(id string) (Result, error) {
	if r == nil {
		return Result{OK: false, Error: "no process registry"}, nil
	}
	r.mu.Lock()
	p, ok := r.procs[id]
	r.mu.Unlock()
	if !ok {
		return Result{OK: false, Error: "unknown process_id: " + id}, nil
	}
	if !p.Running {
		return Result{OK: true, Output: fmt.Sprintf("process %s (pid %d) already exited with code %d", id, p.PID, p.ExitCode)}, nil
	}
	signalProcessGroup(p.PID, syscall.SIGTERM)
	time.Sleep(200 * time.Millisecond)
	signalProcessGroup(p.PID, syscall.SIGKILL)
	r.MarkExited(id, -1)
	return Result{OK: true, Output: fmt.Sprintf("stopped process %s (pid %d)", id, p.PID)}, nil
}

func signalProcessGroup(pid int, sig syscall.Signal) {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		pgid = pid
	}
	_ = syscall.Kill(-pgid, sig)
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Signal(sig)
	}
}

func (r *ProcessRegistry) notify() {
	if r.onChange == nil {
		return
	}
	r.onChange(r.List())
}

// parseTimeoutSec reads timeout_sec from tool args (JSON numbers are float64).
func parseTimeoutSec(args map[string]any) int {
	sec := defaultShellTimeoutSec
	if v, ok := args["timeout_sec"]; ok {
		switch n := v.(type) {
		case float64:
			sec = int(n)
		case int:
			sec = n
		case int64:
			sec = int(n)
		}
	}
	if sec < 1 {
		sec = defaultShellTimeoutSec
	}
	if sec > maxShellTimeoutSec {
		sec = maxShellTimeoutSec
	}
	return sec
}

func parseBoolArg(args map[string]any, key string) bool {
	v, ok := args[key].(bool)
	return ok && v
}