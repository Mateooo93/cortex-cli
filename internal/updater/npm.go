package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

func npmUpdateMessage(tagName string) string {
	return fmt.Sprintf("Updated to %s. Restarting…", tagName)
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


