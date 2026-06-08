package updater

import "testing"

func TestShouldTryNpmUpdate_GitHubPackagesSkipped(t *testing.T) {
	t.Setenv("CORTEX_NPM_PACKAGE", "@mateooo93/cortex")
	if shouldTryNpmUpdate() {
		t.Fatal("expected GitHub Packages install to skip npm update")
	}
}

func TestShouldTryNpmUpdate_LegacyNpmjs(t *testing.T) {
	t.Setenv("CORTEX_NPM_PACKAGE", LegacyNpmPackageName)
	if !shouldTryNpmUpdate() {
		t.Fatal("expected legacy mateooo93-cortex to try npm update")
	}
}

func TestShouldTryNpmUpdate_DefaultPackage(t *testing.T) {
	t.Setenv("CORTEX_NPM_PACKAGE", "")
	if shouldTryNpmUpdate() {
		t.Fatal("expected default @mateooo93/cortex to skip npm update")
	}
}