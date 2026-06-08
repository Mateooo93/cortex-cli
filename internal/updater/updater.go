// Package updater implements cortex-cli's self-update mechanism.
//
// The /update slash command calls Run() which:
//
//  1. Detects the host OS/arch via runtime.GOOS / GOARCH.
//  2. Maps to the matching GitHub release asset name
//     (cortex-darwin-arm64, cortex-linux-amd64, etc.).
//  3. Fetches the latest release metadata from the GitHub API.
//  4. Downloads the asset to a temp file alongside the running
//     binary.
//  5. Verifies its SHA-256 against SHA256SUMS in the same
//     release.
//  6. Renames the running binary to <name>.old, then renames
//     the new binary into place.
//  7. On Windows: spawns a helper process that waits for the
//     current process to exit, then performs the rename (Windows
//     refuses to delete a running executable).
//
// The running process is never overwritten in-place; the user
// must restart for the new binary to take effect. (Replacing a
// running binary on Linux/macOS works but leaves the old code
// mapped into RAM; restarting is the clean answer and matches
// what brew upgrade / rustup update / gh extension upgrade do.)
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Repo is the GitHub owner/repo the updater pulls from. Kept as
// a var (not const) so it can be overridden in tests.
var Repo = "Mateooo93/cortex-cli"

// AssetName maps the host OS/arch to the cortex-cli release
// asset name. The naming convention matches the upload step in
// script/publish.sh — keep them in sync.
func AssetName() (string, error) {
	var name string
	switch runtime.GOOS {
	case "darwin":
		switch runtime.GOARCH {
		case "arm64":
			name = "cortex-darwin-arm64"
		case "amd64":
			name = "cortex-darwin-amd64"
		default:
			return "", fmt.Errorf("updater: unsupported darwin arch %q", runtime.GOARCH)
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			name = "cortex-linux-amd64"
		case "arm64":
			name = "cortex-linux-arm64"
		default:
			return "", fmt.Errorf("updater: unsupported linux arch %q", runtime.GOARCH)
		}
	case "windows":
		switch runtime.GOARCH {
		case "amd64":
			name = "cortex-windows-amd64.exe"
		case "arm64":
			name = "cortex-windows-arm64.exe"
		default:
			return "", fmt.Errorf("updater: unsupported windows arch %q", runtime.GOARCH)
		}
	default:
		return "", fmt.Errorf("updater: unsupported OS %q", runtime.GOOS)
	}
	return name, nil
}

// releaseAsset is the subset of the GitHub releases JSON we need.
type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"` // sha256 digest, e.g. "sha256=abc..."
}

type releaseJSON struct {
	TagName string         `json:"tag_name"`
	Name    string         `json:"name"`
	Assets  []releaseAsset `json:"assets"`
}

// latestRelease fetches the latest release from GitHub.
func latestRelease(ctx context.Context, httpClient *http.Client) (*releaseJSON, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", Repo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "cortex-cli-updater/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("updater: fetch release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("updater: fetch release: status %d: %s", resp.StatusCode, string(body))
	}
	var out releaseJSON
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("updater: decode release: %w", err)
	}
	return &out, nil
}

// findAsset returns the named asset from a release.
func findAsset(rel *releaseJSON, name string) (*releaseAsset, error) {
	for i := range rel.Assets {
		if rel.Assets[i].Name == name {
			return &rel.Assets[i], nil
		}
	}
	return nil, fmt.Errorf("updater: release %s has no asset named %q", rel.TagName, name)
}

// sha256SUMS parses the SHA256SUMS file. Lines look like:
//   <hex>  <filename>
func parseSHA256SUMS(data []byte, filename string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "<hex>  <name>" (two spaces, or * in bin mode).
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		// parts[1] may be "cortex-linux-arm64" or "*cortex-linux-arm64"
		// (binary mode marker).
		name := strings.TrimPrefix(parts[1], "*")
		if name == filename {
			return strings.ToLower(parts[0]), nil
		}
	}
	return "", fmt.Errorf("updater: %s not found in SHA256SUMS", filename)
}

// download downloads url to path. Returns the SHA-256 hex digest
// of the downloaded bytes.
func download(ctx context.Context, httpClient *http.Client, url, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "cortex-cli-updater/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("updater: download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("updater: download: status %d", resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		return "", fmt.Errorf("updater: write: %w", err)
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Run is the entry point. It runs the full update flow and
// returns a result describing what happened so the caller can
// surface it in the TUI status bar.
//
// Result kinds:
//   - "up-to-date" — running version (from version.txt embedded
//     in the binary) matches the latest release tag.
//   - "updated" — new binary installed; user must restart.
//   - "error" — see Result.Error for the reason.
type Result struct {
	Kind       string // "up-to-date" | "updated" | "error"
	OldVersion string
	NewVersion string
	AssetName  string
	OldPath    string
	NewPath    string
	Message    string
	Error      error
}

// Version returns the running binary's version. Wired up to the
// `version` linker var so it can be overridden at build time
// (script/build.sh sets -X main.Version=v0.1.0 etc.).
//
// When unset (e.g. `go run` or a dev build) the updater reports
// "dev" and will always offer to update.
var Version = "dev"

// HTTPClient is the HTTP client used for the API + asset fetch.
// Indirected through a var so tests can plug in a mock server.
var HTTPClient = &http.Client{
	Timeout: 60 * time.Second,
}

// Run performs the update flow synchronously and returns the
// result. The caller should run this in a tea.Cmd goroutine so
// the UI doesn't freeze on the network.
func Run(ctx context.Context) Result {
	return RunWithProgress(ctx, nil)
}

// ProgressFunc is invoked from inside Run() at each meaningful
// step so the UI can render a spinner + step name. The TUI uses
// this to drive the "Checking for updates…" / "Downloading…"
// / "Verifying hash…" / "Installing…" progress messages.
type ProgressFunc func(step string)

// RunWithProgress is Run() with a per-step progress callback.
// Pass nil to disable progress reporting (the CLI subcommand does
// this; only the TUI needs the spinner).
func RunWithProgress(ctx context.Context, progress ProgressFunc) Result {
	if progress != nil {
		progress("Checking for updates…")
	}
	assetName, err := AssetName()
	if err != nil {
		return Result{Kind: "error", Error: err}
	}

	// 1. Fetch latest release metadata.
	if progress != nil {
		progress("Fetching release metadata…")
	}
	rel, err := latestRelease(ctx, HTTPClient)
	if err != nil {
		return Result{Kind: "error", Error: err, AssetName: assetName}
	}

	// 2. Find the asset for our OS/arch.
	asset, err := findAsset(rel, assetName)
	if err != nil {
		return Result{Kind: "error", Error: err, AssetName: assetName, NewVersion: rel.TagName}
	}

	// 3. If we're already on this version, bail early.
	// GitHub tag names are typically "v0.1.0"; strip the "v"
	// prefix before comparing so "v0.1.0" == "0.1.0".
	currentVersion := strings.TrimPrefix(Version, "v")
	latestVersion := strings.TrimPrefix(rel.TagName, "v")
	if currentVersion == latestVersion && currentVersion != "dev" {
		return Result{
			Kind:       "up-to-date",
			OldVersion: currentVersion,
			NewVersion: latestVersion,
			AssetName:  assetName,
			Message:    fmt.Sprintf("cortex-cli %s is already the latest", rel.TagName),
		}
	}

	// 4. Locate the running binary so we know where to write.
	currentExe, err := os.Executable()
	if err != nil {
		return Result{Kind: "error", Error: fmt.Errorf("updater: locate binary: %w", err), NewVersion: rel.TagName}
	}
	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		return Result{Kind: "error", Error: fmt.Errorf("updater: resolve symlinks: %w", err), NewVersion: rel.TagName}
	}

	// npm installs: for the legacy npmjs.org package, try `npm update -g`
	// (bounded timeout). GitHub Packages installs skip npm entirely — npm
	// update against npm.pkg.github.com hangs without registry auth — and
	// fall through to a direct GitHub release download into ~/.cortex/npm/.
	if IsNpmInstall(currentExe) && shouldTryNpmUpdate() {
		if progress != nil {
			progress(fmt.Sprintf("Updating %s via npm…", npmPackageName()))
		}
		if npmErr := npmUpdate(ctx); npmErr == nil {
			restart, rerr := restartPathForNpm()
			if rerr != nil {
				return Result{Kind: "error", Error: rerr, NewVersion: rel.TagName, AssetName: assetName}
			}
			return Result{
				Kind:       "updated",
				OldVersion: currentVersion,
				NewVersion: latestVersion,
				AssetName:  assetName,
				OldPath:    currentExe,
				NewPath:    restart,
				Message:    npmInstallMessage(rel.TagName),
			}
		} else if progress != nil {
			progress("npm update failed — downloading release directly…")
		}
	} else if IsNpmInstall(currentExe) && progress != nil {
		progress(fmt.Sprintf("Downloading %s…", rel.TagName))
	}

	// 5. Plan where to install. If the running binary's directory
	// isn't writable (system-wide install, read-only mount, etc.)
	// we fall back to ~/.local/bin on the install step.
	plan, err := planInstall(currentExe)
	if err != nil {
		return Result{Kind: "error", Error: err, NewVersion: rel.TagName, AssetName: assetName}
	}

	// Download to the system temp dir so we never need write access
	// beside the running binary just to fetch the release asset.
	tmpPath, cleanupTemp, err := createDownloadTemp(assetName)
	if err != nil {
		return Result{Kind: "error", Error: err, NewVersion: rel.TagName, AssetName: assetName}
	}
	defer cleanupTemp()
	if progress != nil {
		progress(fmt.Sprintf("Downloading %s…", rel.TagName))
	}
	downloadedHash, err := download(ctx, HTTPClient, asset.BrowserDownloadURL, tmpPath)
	if err != nil {
		return Result{Kind: "error", Error: err, NewVersion: rel.TagName, AssetName: assetName}
	}

	// 6. Verify SHA-256 against SHA256SUMS in the same release.
	if progress != nil {
		progress("Verifying SHA-256…")
	}
	sumsAsset, err := findAsset(rel, "SHA256SUMS")
	if err != nil {
		// No SHA256SUMS — that's a release bug, not a download
		// bug, so we fail safe.
		_ = os.RemoveAll(tmpPath)
		return Result{Kind: "error", Error: fmt.Errorf("updater: release %s has no SHA256SUMS asset; refusing to install", rel.TagName), NewVersion: rel.TagName, AssetName: assetName}
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", sumsAsset.BrowserDownloadURL, nil)
	req.Header.Set("User-Agent", "cortex-cli-updater/1.0")
	resp, err := HTTPClient.Do(req)
	if err != nil {
		_ = os.RemoveAll(tmpPath)
		return Result{Kind: "error", Error: fmt.Errorf("updater: fetch SHA256SUMS: %w", err), NewVersion: rel.TagName, AssetName: assetName}
	}
	defer resp.Body.Close()
	sumsBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = os.RemoveAll(tmpPath)
		return Result{Kind: "error", Error: err, NewVersion: rel.TagName, AssetName: assetName}
	}
	expectedHash, err := parseSHA256SUMS(sumsBytes, assetName)
	if err != nil {
		_ = os.RemoveAll(tmpPath)
		return Result{Kind: "error", Error: err, NewVersion: rel.TagName, AssetName: assetName}
	}
	if !strings.EqualFold(downloadedHash, expectedHash) {
		_ = os.RemoveAll(tmpPath)
		return Result{Kind: "error", Error: fmt.Errorf("updater: hash mismatch: downloaded %s, expected %s", downloadedHash[:12]+"...", expectedHash[:12]+"..."), NewVersion: rel.TagName, AssetName: assetName}
	}

	// 7. Install the verified binary.
	if progress != nil {
		progress("Installing new binary…")
	}

	// On Windows you can't rename a running executable in place.
	// Fall back to the helper-process flow when we're replacing
	// the running binary directly.
	if runtime.GOOS == "windows" && plan.inPlace && plan.targetPath == currentExe {
		if winErr := installOnWindows(tmpPath, currentExe); winErr != nil {
			return Result{Kind: "error", Error: winErr, NewVersion: rel.TagName, AssetName: assetName}
		}
		return Result{
			Kind:       "updated",
			OldVersion: currentVersion,
			NewVersion: latestVersion,
			AssetName:  assetName,
			OldPath:    currentExe,
			NewPath:    currentExe,
			Message:    installMessage(rel.TagName, plan),
		}
	}

	finalPlan, err := applyInstall(tmpPath, plan)
	if err != nil {
		if isPermissionErr(err) {
			return Result{
				Kind:       "error",
				Error:      fmt.Errorf("updater: cannot install update (%w). Try moving cortex to ~/.local/bin or ensure that directory is in your PATH", err),
				NewVersion: rel.TagName,
				AssetName:  assetName,
			}
		}
		return Result{Kind: "error", Error: err, NewVersion: rel.TagName, AssetName: assetName}
	}

	oldPath := ""
	if finalPlan.inPlace {
		oldPath = finalPlan.targetPath + ".old"
	}

	return Result{
		Kind:       "updated",
		OldVersion: currentVersion,
		NewVersion: latestVersion,
		AssetName:  assetName,
		OldPath:    oldPath,
		NewPath:    finalPlan.targetPath,
		Message:    installMessage(rel.TagName, finalPlan),
	}
}

// installOnWindows is the Windows-specific install path. Windows
// refuses to rename/delete a running executable, so we spawn a
// helper process that waits for us to exit, then performs the
// swap. The helper is itself a copy of the new binary (cortex is
// a static Go binary, so it doesn't care what it's called).
//
//   1. tmpPath = downloaded new binary
//   2. helperPath = a copy of tmpPath in the same directory
//   3. Spawn `helperPath <currentExe> <tmpPath>` as a detached
//      process that waits for currentExe to exit, then moves
//      tmpPath -> currentExe.
//
// We pass the helper the same args we'd give the real binary; it
// just uses argv[1] / argv[2] to know what to do.
func installOnWindows(tmpPath, currentExe string) error {
	helperPath := tmpPath + ".helper"
	if err := copyFile(tmpPath, helperPath); err != nil {
		return fmt.Errorf("updater: copy helper: %w", err)
	}
	// Detach: CREATE_NEW_PROCESS_GROUP so the helper doesn't
	// get killed when we exit.
	cmd := exec.Command(helperPath, "__update_helper__", currentExe, tmpPath)
	cmd.SysProcAttr = windowsDetachedAttrs()
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(helperPath)
		return fmt.Errorf("updater: spawn helper: %w", err)
	}
	// Don't Wait — the helper does its work after we exit.
	go func() { _ = cmd.Wait() }()
	return nil
}

// ErrUnsupportedArch is returned by AssetName when there's no
// pre-built binary for the current OS/arch combination.
var ErrUnsupportedArch = errors.New("updater: no pre-built release for this OS/arch")
