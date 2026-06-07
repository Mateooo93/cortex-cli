package tools

import (
	"os"
	"path/filepath"
	"strings"
)

// resolvePath normalizes a path string the LLM might
// pass to a file tool (write_file, edit_file, read_file,
// delete_file). The user-reported bug: the agent wrote
//
//   "path": "home/ubuntu/prixm/prixm/routers/status.py"
//
// expecting it to mean "/home/ubuntu/prixm/prixm/routers/
// status.py" but the tool interpreted it as a path
// relative to CWD, creating
//
//   {cwd}/home/ubuntu/prixm/prixm/routers/status.py
//
// (i.e. /home/ubuntu/prixmwebsite/home/ubuntu/prixm/prixm/
// routers/status.py when CWD is /home/ubuntu/prixmwebsite).
//
// The LLM forgets the leading slash on absolute paths
// surprisingly often. Rather than fail the tool call,
// we auto-correct: if the path LOOKS like it was meant
// to be absolute (starts with a known absolute root
// like "home/", "Users/", "tmp/", "var/", "opt/", "etc/"),
// we prepend "/" so it becomes a proper absolute path.
// We also expand "~/" to the user's home directory.
//
// The corrected path is returned along with a boolean
// indicating whether auto-correction happened, so the
// tool result can include a note in the output (e.g.
// "wrote to /home/ubuntu/... (auto-corrected from
// home/ubuntu/...)") — that way the LLM sees what
// actually happened and can learn from the correction
// on subsequent calls.
//
// Real relative paths (e.g. "./foo.txt", "src/main.go")
// are left alone — we only correct "looks-absolute-
// missing-leading-slash" patterns, which the LLM
// produces by far the most often.
func resolvePath(cwd, p string) (resolved string, corrected bool) {
	if p == "" {
		return p, false
	}
	// Already absolute (POSIX or Windows): no
	// correction needed.
	if filepath.IsAbs(p) {
		// Expand ~ even on absolute paths.
		if strings.HasPrefix(p, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				return filepath.Join(home, strings.TrimPrefix(p, "~")), true
			}
		}
		return p, false
	}
	// Expand ~ to home directory.
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return p, false
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~")), true
	}
	// Looks-absolute-but-missing-leading-slash.
	// We use a small set of common absolute-path
	// roots that the LLM forgets the "/" for. The
	// set is conservative: each entry is something
	// that ONLY makes sense as an absolute path on
	// the host (i.e. would never legitimately be a
	// relative path in a real project). If the
	// project happens to be a sibling of /home
	// (very rare), the user can always pass the
	// path with an explicit "./" prefix to opt out
	// of auto-correction.
	for _, root := range []string{
		"home/",
		"root/",
		"Users/",
		"tmp/",
		"var/",
		"opt/",
		"etc/",
		"srv/",
		"mnt/",
		"data/",
	} {
		if strings.HasPrefix(p, root) {
			return "/" + p, true
		}
	}
	// Genuinely relative path: leave alone.
	return filepath.Join(cwd, p), false
}
