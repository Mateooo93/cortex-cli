package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NpmPackageName is the global npm package published for cortex-cli.
const NpmPackageName = "mateooo93-cortex"

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

func npmUpdate(ctx context.Context) error {
	npm, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("updater: npm not found on PATH")
	}
	cmd := exec.CommandContext(ctx, npm, "update", "-g", NpmPackageName)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("updater: npm update -g %s failed: %w", NpmPackageName, err)
		}
		return fmt.Errorf("updater: npm update -g %s failed: %s", NpmPackageName, msg)
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
	return fmt.Sprintf("Updated %s to %s via npm. Restarting…", NpmPackageName, tagName)
}