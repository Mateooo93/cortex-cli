package updater

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNpmCacheBinaryPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := npmCacheBinaryPath("v0.25.22", "cortex-linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".cortex", "npm", "0.25.22", "cortex-linux-amd64")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if st, err := os.Stat(filepath.Dir(got)); err != nil || !st.IsDir() {
		t.Fatalf("expected version dir created: %v", err)
	}
}

func TestPlanNpmInstall_TargetsVersionedCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	currentExe := filepath.Join(home, ".cortex", "npm", "0.25.20", "cortex-linux-amd64")

	plan, err := planNpmInstall("0.25.22", "cortex-linux-amd64", currentExe)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".cortex", "npm", "0.25.22", "cortex-linux-amd64")
	if plan.targetPath != want {
		t.Fatalf("targetPath = %q, want %q", plan.targetPath, want)
	}
	if plan.inPlace {
		t.Fatal("expected non-in-place npm install plan")
	}
}

func TestUpdateNpmCurrentSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	target := filepath.Join(home, ".cortex", "npm", "0.25.22", "cortex-linux-amd64")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := updateNpmCurrentSymlink(target, "cortex"); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".cortex", "npm", "current", "cortex")
	resolved, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != target {
		t.Fatalf("symlink = %q, want %q", resolved, target)
	}
}


