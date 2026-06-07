package tools

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestResolvePath_AbsoluteUnchanged pins the
// no-op behaviour: a path that already starts with
// "/" is returned as-is (with the corrected flag
// false).
func TestResolvePath_AbsoluteUnchanged(t *testing.T) {
	got, corrected := resolvePath("/home/ubuntu/prixm", "/etc/nginx/nginx.conf")
	if got != "/etc/nginx/nginx.conf" {
		t.Errorf("expected unchanged path, got %q", got)
	}
	if corrected {
		t.Errorf("expected corrected=false, got true")
	}
}

// TestResolvePath_RemainsRelative pins the
// no-correction case: a genuine relative path is
// joined to the CWD without modification.
func TestResolvePath_RemainsRelative(t *testing.T) {
	got, corrected := resolvePath("/home/ubuntu/prixm", "src/main.go")
	if got != filepath.Join("/home/ubuntu/prixm", "src/main.go") {
		t.Errorf("expected CWD-joined path, got %q", got)
	}
	if corrected {
		t.Errorf("expected corrected=false for genuine relative, got true")
	}
}

// TestResolvePath_AutoCorrectHomeMissingSlash is
// the user-reported bug: the agent wrote
// "home/ubuntu/foo.py" expecting it to be
// "/home/ubuntu/foo.py" but the tool created
// "{cwd}/home/ubuntu/foo.py" instead. The fix
// auto-corrects by prepending "/".
func TestResolvePath_AutoCorrectHomeMissingSlash(t *testing.T) {
	got, corrected := resolvePath("/home/ubuntu/prixm", "home/ubuntu/foo.py")
	if got != "/home/ubuntu/foo.py" {
		t.Errorf("expected /home/ubuntu/foo.py, got %q", got)
	}
	if !corrected {
		t.Errorf("expected corrected=true for missing-leading-slash, got false")
	}
}

// TestResolvePath_AutoCorrectAllKnownRoots pins
// the full set of "looks-absolute" roots we
// auto-correct. Each is a path that ONLY makes
// sense as an absolute path on the host.
func TestResolvePath_AutoCorrectAllKnownRoots(t *testing.T) {
	roots := []string{
		"home/ubuntu/file.txt",
		"root/.bashrc",
		"Users/alice/Documents/x.md",
		"tmp/cache.bin",
		"var/log/app.log",
		"opt/app/config.yaml",
		"etc/hosts",
		"srv/data/x.json",
		"mnt/external/x",
		"data/something",
	}
	for _, p := range roots {
		got, corrected := resolvePath("/cwd", p)
		if !corrected {
			t.Errorf("%q: expected corrected=true", p)
		}
		if !strings.HasPrefix(got, "/") {
			t.Errorf("%q: expected corrected path to start with /, got %q", p, got)
		}
		if got == "/"+p {
			// good — auto-corrected by prepending /
		} else {
			t.Errorf("%q: expected /%q, got %q", p, p, got)
		}
	}
}

// TestResolvePath_OptOutWithDotSlash pins the
// escape hatch: a user can prefix their relative
// path with "./" to opt out of auto-correction
// (useful when the project legitimately has a
// "home/" or "tmp/" subdirectory).
func TestResolvePath_OptOutWithDotSlash(t *testing.T) {
	got, corrected := resolvePath("/cwd", "./home/foo.txt")
	if corrected {
		t.Errorf("expected corrected=false for explicit relative, got true")
	}
	if got != filepath.Join("/cwd", "home/foo.txt") {
		t.Errorf("expected CWD-joined path, got %q", got)
	}
}

// TestResolvePath_TildeExpanded pins ~ expansion
// (works on absolute or relative ~).
func TestResolvePath_TildeExpanded(t *testing.T) {
	got, corrected := resolvePath("/cwd", "~/notes.txt")
	if !corrected {
		t.Errorf("expected corrected=true for ~ expansion, got false")
	}
	if !strings.HasSuffix(got, "/notes.txt") {
		t.Errorf("expected path to end in /notes.txt, got %q", got)
	}
	// Should NOT be the literal "~/notes.txt".
	if got == "~/notes.txt" {
		t.Errorf("~ should have been expanded, got %q", got)
	}
}
