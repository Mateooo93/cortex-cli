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

// npmInstallSpec is the package spec passed to npm install -g for /update.
func npmInstallSpec() string {
	return npmPackageName() + "@latest"
}

// npmInstallLatest runs npm install -g @mateooo93/cortex@latest (or the
// package named by CORTEX_NPM_PACKAGE).
func npmInstallLatest(ctx context.Context) error {
	npm, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("updater: npm not found on PATH")
	}
	spec := npmInstallSpec()
	ctx, cancel := context.WithTimeout(ctx, npmUpdateTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, npm, "install", "-g", spec)
	cmd.Env = os.Environ()
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


