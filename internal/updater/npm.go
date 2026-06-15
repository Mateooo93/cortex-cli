package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

// RepairNpmWrapperCache keeps old already-installed npm wrappers from
// relaunching their package-pinned native cache after /update.
//
// Older npm wrappers always launch ~/.cortex/npm/<package-version>/<asset> and
// verify that path by running it with --version before each launch. After
// /update, the fresh binary lives in ~/.cortex/npm/<latest>/<asset>, so closing
// and reopening the CLI would fall back to the old package cache. The repair
// replaces that stale cache entry with a small launcher:
//   - old wrapper's internal --version probe still sees the package version
//   - normal launches exec the freshly updated binary
func RepairNpmWrapperCache() error {
	if strings.TrimSpace(os.Getenv("CORTEX_NPM_PACKAGE")) == "" {
		return nil
	}
	if runtime.GOOS == "windows" {
		return nil
	}

	pkgVersion := npmPackageVersionFromShim()
	if pkgVersion == "" || pkgVersion == "0.0.0-dev" {
		return nil
	}

	assetName, err := AssetName()
	if err != nil {
		return err
	}
	stalePath, err := npmCacheBinaryPath(pkgVersion, assetName)
	if err != nil {
		return err
	}

	currentExe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(currentExe); err == nil {
		currentExe = resolved
	}
	if currentExe == stalePath {
		return nil
	}

	targetVersion := strings.TrimPrefix(strings.TrimSpace(Version), "v")
	if targetVersion == "" || targetVersion == "dev" || targetVersion == pkgVersion {
		return nil
	}

	return writeNpmCompatibilityLauncher(stalePath, currentExe, pkgVersion)
}

func npmPackageVersionFromShim() string {
	shim := strings.TrimSpace(os.Getenv("CORTEX_NPM_SHIM"))
	if shim == "" {
		return ""
	}
	pkgPath := filepath.Join(filepath.Dir(filepath.Dir(shim)), "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return ""
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(pkg.Version), "v")
}

func writeNpmCompatibilityLauncher(path, targetPath, probeVersion string) error {
	backup := path + ".native-old"
	_ = os.Remove(backup)
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backup); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	script := "#!/bin/sh\n" +
		"target=" + shellSingleQuote(targetPath) + "\n" +
		"probe_version=" + shellSingleQuote(probeVersion) + "\n" +
		"case \"${1:-}\" in\n" +
		"  --version|-version|version)\n" +
		"    if [ -z \"${CORTEX_NPM_PACKAGE:-}\" ]; then\n" +
		"      printf 'cortex %s\\n' \"$probe_version\"\n" +
		"      exit 0\n" +
		"    fi\n" +
		"    ;;\n" +
		"esac\n" +
		"exec \"$target\" \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		_ = os.Rename(backup, path)
		return err
	}
	return nil
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
