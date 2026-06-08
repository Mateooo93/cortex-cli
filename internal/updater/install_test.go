package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanInstall_WritableDirUsesInPlace(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "cortex")
	plan, err := planInstall(exe)
	if err != nil {
		t.Fatalf("planInstall: %v", err)
	}
	if !plan.inPlace || plan.targetPath != exe {
		t.Fatalf("plan = %+v, want in-place install to %s", plan, exe)
	}
}

func TestPlanInstall_ReadOnlyDirFallsBackToUserBin(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skipf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	home := t.TempDir()
	t.Setenv("HOME", home)

	exe := filepath.Join(dir, "cortex")
	plan, err := planInstall(exe)
	if err != nil {
		t.Fatalf("planInstall: %v", err)
	}
	want := filepath.Join(home, ".local", "bin", installBinaryName())
	if plan.inPlace || plan.targetPath != want {
		t.Fatalf("plan = %+v, want user-bin install to %s", plan, want)
	}
}

func TestApplyInstall_UserBinFromReadOnlySource(t *testing.T) {
	readOnlyDir := t.TempDir()
	if err := os.Chmod(readOnlyDir, 0o555); err != nil {
		t.Skipf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnlyDir, 0o755) })

	home := t.TempDir()
	t.Setenv("HOME", home)

	currentExe := filepath.Join(readOnlyDir, "cortex")
	plan, err := planInstall(currentExe)
	if err != nil {
		t.Fatalf("planInstall: %v", err)
	}

	tmp := filepath.Join(t.TempDir(), "cortex.new")
	if err := os.WriteFile(tmp, []byte("#!/bin/sh\necho updated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	finalPlan, err := applyInstall(tmp, plan)
	if err != nil {
		t.Fatalf("applyInstall: %v", err)
	}
	if _, err := os.Stat(finalPlan.targetPath); err != nil {
		t.Fatalf("installed binary missing at %s: %v", finalPlan.targetPath, err)
	}
	msg := installMessage("v1.2.3", finalPlan)
	if !strings.Contains(msg, finalPlan.targetPath) {
		t.Fatalf("expected path in message, got %q", msg)
	}
}