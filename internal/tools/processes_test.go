package tools

import (
	"strings"
	"testing"
	"time"
)

func TestShellTool_BackgroundReturnsImmediately(t *testing.T) {
	reg := NewProcessRegistry(nil)
	tctx := Context{CWD: t.TempDir(), AllowShell: true, Processes: reg}
	start := time.Now()
	res, err := (&ShellTool{}).Run(tctx, map[string]any{
		"command":    "sleep 3",
		"background": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected OK, got error %q", res.Error)
	}
	if time.Since(start) > 800*time.Millisecond {
		t.Fatalf("background command should return immediately, took %v", time.Since(start))
	}
	if !strings.Contains(res.Output, "background") {
		t.Fatalf("expected background message, got %q", res.Output)
	}
	procs := reg.List()
	if len(procs) != 1 || !procs[0].Running {
		t.Fatalf("expected one running process, got %+v", procs)
	}
	// Wait for sleep to finish and registry to update.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		procs = reg.List()
		if len(procs) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("background process did not exit in time")
}

func TestShellTool_TimeoutDetachesWithoutKill(t *testing.T) {
	reg := NewProcessRegistry(nil)
	tctx := Context{CWD: t.TempDir(), AllowShell: true, Processes: reg}
	res, err := (&ShellTool{}).Run(tctx, map[string]any{
		"command":     "sleep 5",
		"timeout_sec": float64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected detach OK, got error %q", res.Error)
	}
	if !strings.Contains(res.Output, "Detached") {
		t.Fatalf("expected detach message, got %q", res.Output)
	}
	procs := reg.List()
	if len(procs) != 1 || !procs[0].Running {
		t.Fatalf("expected detached running process, got %+v", procs)
	}
}

func TestShellTool_BackgroundShellAmpersandStaysTracked(t *testing.T) {
	reg := NewProcessRegistry(nil)
	tctx := Context{CWD: t.TempDir(), AllowShell: true, Processes: reg}
	res, err := (&ShellTool{}).Run(tctx, map[string]any{
		"command":    "sleep 4 &",
		"background": true,
	})
	if err != nil || !res.OK {
		t.Fatalf("start: err=%v res=%+v", err, res)
	}
	time.Sleep(200 * time.Millisecond)
	procs := reg.List()
	if len(procs) != 1 || !procs[0].Running {
		t.Fatalf("expected tracked process still running after shell &, got %+v", procs)
	}
}

func TestProcessRegistry_Stop(t *testing.T) {
	reg := NewProcessRegistry(nil)
	tctx := Context{CWD: t.TempDir(), AllowShell: true, Processes: reg}
	res, err := (&ShellTool{}).Run(tctx, map[string]any{
		"command":    "sleep 60",
		"background": true,
	})
	if err != nil || !res.OK {
		t.Fatalf("start: err=%v res=%+v", err, res)
	}
	id, _ := res.Details["process_id"].(string)
	if id == "" {
		t.Fatal("expected process_id in details")
	}
	stopRes, err := (&StopBackgroundProcessTool{}).Run(tctx, map[string]any{"process_id": id})
	if err != nil || !stopRes.OK {
		t.Fatalf("stop: err=%v res=%+v", err, stopRes)
	}
	procs := reg.List()
	if len(procs) != 0 {
		t.Fatalf("expected process removed after stop, got %+v", procs)
	}
}

func TestProcessRegistry_MarkExitedRemovesProcess(t *testing.T) {
	reg := NewProcessRegistry(nil)
	id := reg.Register(1234, "sleep 1", "/tmp")
	if id == "" {
		t.Fatal("expected process id")
	}
	reg.MarkExited(id, 0)
	if len(reg.List()) != 0 {
		t.Fatalf("expected empty registry, got %+v", reg.List())
	}
}