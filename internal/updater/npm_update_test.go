package updater

import "testing"

func TestNpmInstallSpec(t *testing.T) {
	t.Setenv("CORTEX_NPM_PACKAGE", "")
	if got := npmInstallSpec(); got != "@mateooo93/cortex@latest" {
		t.Fatalf("spec = %q, want @mateooo93/cortex@latest", got)
	}
}

func TestNpmInstallSpec_EnvOverride(t *testing.T) {
	t.Setenv("CORTEX_NPM_PACKAGE", LegacyNpmPackageName)
	if got := npmInstallSpec(); got != LegacyNpmPackageName+"@latest" {
		t.Fatalf("spec = %q, want %s@latest", got, LegacyNpmPackageName)
	}
}