package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const npmUpdateTimeout = 90 * time.Second

// NpmPackageName is the GitHub Packages npm wrapper for cortex-cli.
const NpmPackageName = "@mateooo93/cortex"

// NpmPackageRegistry is where @mateooo93/cortex is published.
const NpmPackageRegistry = "https://npm.pkg.github.com"

// LegacyNpmPackageName is the old unscoped npmjs.org package (pre-GH Packages).
const LegacyNpmPackageName = "mateooo93-cortex"

// IsNpmInstall reports whether cortex was launched from the npm wrapper.
// The npm shim sets CORTEX_NPM_PACKAGE; cached binaries live under
// ~/.cortex/npm/.
func IsNpmInstall(exe string) bool {
	if strings.TrimSpace(os.Getenv("CORTEX_NPM_PACKAGE")) != "" {
		return true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	cache := filepath.Join(home, ".cortex", "npm")
	exe = filepath.Clean(exe)
	return strings.HasPrefix(exe, cache+string(filepath.Separator))
}

func npmPackageName() string {
	if n := strings.TrimSpace(os.Getenv("CORTEX_NPM_PACKAGE")); n != "" {
		return n
	}
	return NpmPackageName
}

// shouldTryNpmUpdate reports whether /update should shell out to npm.
// GitHub Packages (@mateooo93/*) requires registry auth for npm update and
// commonly hangs in headless installs — we download the GitHub release
// binary directly into ~/.cortex/npm/ instead.
func shouldTryNpmUpdate() bool {
	pkg := npmPackageName()
	if strings.HasPrefix(pkg, "@mateooo93/") {
		return false
	}
	// Legacy unscoped package on npmjs.org.
	return pkg == LegacyNpmPackageName
}

func npmUpdate(ctx context.Context) error {
	npm, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("updater: npm not found on PATH")
	}
	pkg := npmPackageName()
	ctx, cancel := context.WithTimeout(ctx, npmUpdateTimeout)
	defer cancel()
	args := []string{"update", "-g", pkg}
	cmd := exec.CommandContext(ctx, npm, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("updater: npm update -g %s failed: %w", pkg, err)
		}
		return fmt.Errorf("updater: npm update -g %s failed: %s", pkg, msg)
	}
	return nil
}

// restartPathForNpm returns the cortex launcher to re-exec after an npm
// update. Prefer the global npm bin wrapper over the raw .js shim path.
func restartPathForNpm() (string, error) {
	if path, err := exec.LookPath("cortex"); err == nil {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		return path, nil
	}
	if shim := strings.TrimSpace(os.Getenv("CORTEX_NPM_SHIM")); shim != "" {
		if resolved, err := filepath.EvalSymlinks(shim); err == nil {
			shim = resolved
		}
		return shim, nil
	}
	return "", fmt.Errorf("updater: locate cortex on PATH after npm update")
}

func npmInstallMessage(tagName string) string {
	return fmt.Sprintf("Updated %s to %s via npm. Restarting…", npmPackageName(), tagName)
}

// npmCacheBinaryPath returns ~/.cortex/npm/<version>/<asset>, creating the
// version directory when needed. Matches the layout used by postinstall.
func npmCacheBinaryPath(version, assetName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	ver := strings.TrimPrefix(strings.TrimSpace(version), "v")
	dir := filepath.Join(home, ".cortex", "npm", ver)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, assetName), nil
}

// planNpmInstall targets the versioned npm cache path for the latest release
// instead of replacing the running binary in an older cache directory.
func planNpmInstall(version, assetName, currentExe string) (installPlan, error) {
	target, err := npmCacheBinaryPath(version, assetName)
	if err != nil {
		return installPlan{}, err
	}
	return installPlan{
		targetPath: target,
		inPlace:    false,
		currentExe: currentExe,
		sourceDir:  filepath.Dir(currentExe),
	}, nil
}

// updateNpmCurrentSymlink points ~/.cortex/npm/current/<binaryName> at the
// installed native binary so other tooling can find the active build.
func updateNpmCurrentSymlink(targetPath, binaryName string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	currentDir := filepath.Join(home, ".cortex", "npm", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		return err
	}
	linkPath := filepath.Join(currentDir, binaryName)
	_ = os.Remove(linkPath)
	return os.Symlink(targetPath, linkPath)
}

// npmInstallGitHub runs a bounded global npm install for the GitHub Packages
// wrapper so readPackageVersion() and postinstall stay aligned with the
// native binary after a direct /update download. Best-effort: failures are
// ignored by the caller.
func npmInstallGitHub(ctx context.Context, tagName string) error {
	npm, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("updater: npm not found on PATH")
	}
	pkg := npmPackageName()
	ver := strings.TrimPrefix(strings.TrimSpace(tagName), "v")
	ctx, cancel := context.WithTimeout(ctx, npmUpdateTimeout)
	defer cancel()
	spec := pkg + "@" + ver
	args := []string{
		"install", "-g", spec,
		"--registry", NpmPackageRegistry,
		"--prefer-online",
		"--no-fund",
		"--no-audit",
	}
	cmd := exec.CommandContext(ctx, npm, args...)
	env := os.Environ()
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" && strings.TrimSpace(os.Getenv("NODE_AUTH_TOKEN")) == "" {
		env = append(env, "NODE_AUTH_TOKEN="+token)
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("updater: npm install -g %s failed: %w", spec, err)
		}
		return fmt.Errorf("updater: npm install -g %s failed: %s", spec, msg)
	}
	return nil
}
