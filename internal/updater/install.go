package updater

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// installPlan describes where the updater should write the new
// binary. When the running executable lives in a directory the
// user cannot write (e.g. /usr/local/bin, a read-only AppImage
// mount, or a root-owned path), we fall back to ~/.local/bin.
type installPlan struct {
	targetPath string
	inPlace    bool
	currentExe string
	sourceDir  string
}

func installBinaryName() string {
	if runtime.GOOS == "windows" {
		return "cortex.exe"
	}
	return "cortex"
}

func userLocalBinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// dirWritable reports whether the user can create files in dir.
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".cortex-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func planInstall(currentExe string) (installPlan, error) {
	dir := filepath.Dir(currentExe)
	if dirWritable(dir) {
		return installPlan{
			targetPath: currentExe,
			inPlace:    true,
			currentExe: currentExe,
			sourceDir:  dir,
		}, nil
	}
	return planUserInstall(currentExe)
}

func planUserInstall(currentExe string) (installPlan, error) {
	userDir, err := userLocalBinDir()
	if err != nil {
		return installPlan{}, err
	}
	if !dirWritable(userDir) {
		return installPlan{}, fmt.Errorf("updater: no writable install directory (tried %s and %s)", filepath.Dir(currentExe), userDir)
	}
	return installPlan{
		targetPath: filepath.Join(userDir, installBinaryName()),
		inPlace:    false,
		currentExe: currentExe,
		sourceDir:  filepath.Dir(currentExe),
	}, nil
}

func isPermissionErr(err error) bool {
	if err == nil {
		return false
	}
	if os.IsPermission(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission denied") || strings.Contains(msg, "operation not permitted")
}

// moveReplace moves src onto dst, removing dst first when needed.
// Falls back to copy when src and dst are on different filesystems.
func moveReplace(src, dst string) error {
	_ = os.Remove(dst)
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if copyErr := copyFile(src, dst); copyErr != nil {
		return fmt.Errorf("rename: %w; copy: %v", err, copyErr)
	}
	_ = os.Remove(src)
	return nil
}

func applyInstall(tmpPath string, plan installPlan) (installPlan, error) {
	if err := tryInstall(tmpPath, plan); err == nil {
		return plan, nil
	} else if plan.inPlace && isPermissionErr(err) {
		fallback, fbErr := planUserInstall(plan.currentExe)
		if fbErr != nil {
			return plan, err
		}
		if fbErr := tryInstall(tmpPath, fallback); fbErr != nil {
			return plan, fmt.Errorf("%w; user install to %s also failed: %v", err, fallback.targetPath, fbErr)
		}
		return fallback, nil
	} else {
		return plan, err
	}
}

func tryInstall(tmpPath string, plan installPlan) error {
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			return err
		}
	}

	if plan.inPlace {
		oldPath := plan.targetPath + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(plan.currentExe, oldPath); err != nil {
			return err
		}
		if err := moveReplace(tmpPath, plan.targetPath); err != nil {
			_ = os.Rename(oldPath, plan.currentExe)
			return err
		}
		return nil
	}

	if err := moveReplace(tmpPath, plan.targetPath); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Chmod(plan.targetPath, 0o755)
	}
	return nil
}

func installMessage(tagName string, plan installPlan) string {
	if plan.inPlace {
		return fmt.Sprintf("Updated to %s. Restarting…", tagName)
	}
	msg := fmt.Sprintf("Updated to %s at %s. Restarting…", tagName, plan.targetPath)
	if plan.sourceDir != filepath.Dir(plan.targetPath) {
		msg += fmt.Sprintf(" (could not write to %s; ensure ~/.local/bin is in your PATH)", plan.sourceDir)
	}
	return msg
}

// createDownloadTemp opens a writable temp file for the downloaded
// release asset. Using the system temp dir avoids permission errors
// when the running binary lives in a root-owned directory.
func createDownloadTemp(assetName string) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "cortex-update-"+assetName+"-*")
	if err != nil {
		return "", nil, err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", nil, err
	}
	return name, func() { _ = os.Remove(name) }, nil
}

// copyFile duplicates src to dst. Exported internally for Windows
// helper and cross-device installs.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}