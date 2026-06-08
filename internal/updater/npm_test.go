package updater

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestIsNpmInstall_Env(t *testing.T) {
	t.Setenv("CORTEX_NPM_PACKAGE", NpmPackageName)
	if !IsNpmInstall("/usr/local/bin/cortex") {
		t.Fatal("expected npm install when CORTEX_NPM_PACKAGE is set")
	}
}

func TestIsNpmInstall_CachePath(t *testing.T) {
	t.Setenv("CORTEX_NPM_PACKAGE", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	cacheBin := filepath.Join(home, ".cortex", "npm", "0.25.18", "cortex-linux-amd64")
	if !IsNpmInstall(cacheBin) {
		t.Fatalf("expected npm install for cache path %s", cacheBin)
	}
	if IsNpmInstall("/usr/local/bin/cortex") {
		t.Fatal("expected non-npm path to return false")
	}
}

func TestNpmInstallMessage(t *testing.T) {
	msg := npmInstallMessage("v0.25.19")
	for _, want := range []string{"mateooo93-cortex", "v0.25.19", "npm"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q missing %q", msg, want)
		}
	}
}