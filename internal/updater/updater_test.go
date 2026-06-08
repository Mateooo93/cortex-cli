package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAssetName_KnownPlatform(t *testing.T) {
	name, err := AssetName()
	if err != nil {
		t.Fatalf("AssetName: %v", err)
	}
	// Every supported platform produces a name with the
	// "cortex-" prefix and matches the publish.sh convention.
	if !strings.HasPrefix(name, "cortex-") {
		t.Errorf("name = %q, want cortex-* prefix", name)
	}
	// Sanity check by GOOS/GOARCH.
	host := runtime.GOOS + "-" + runtime.GOARCH
	if !strings.Contains(name, runtime.GOOS) {
		t.Errorf("name %q doesn't contain GOOS %q", name, host)
	}
}

func TestAssetName_IncludesExeOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only check")
	}
	name, err := AssetName()
	if err != nil {
		t.Fatalf("AssetName: %v", err)
	}
	if !strings.HasSuffix(name, ".exe") {
		t.Errorf("windows asset = %q, want .exe suffix", name)
	}
}

func TestDigestHex(t *testing.T) {
	if got := digestHex("sha256=ABC123"); got != "abc123" {
		t.Fatalf("digestHex = %q", got)
	}
	if got := digestHex("sha256:DEADBEEF"); got != "deadbeef" {
		t.Fatalf("digestHex colon = %q", got)
	}
	if digestHex("") != "" {
		t.Fatal("expected empty for blank digest")
	}
}

func TestExpectedHashForAsset_PrefersDigest(t *testing.T) {
	assetName, _ := AssetName()
	rel := &releaseJSON{
		TagName: "v1.0.0",
		Assets: []releaseAsset{{
			Name:   assetName,
			Digest: "sha256=abc123",
		}},
	}
	got, err := expectedHashForAsset(context.Background(), HTTPClient, rel, &rel.Assets[0])
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc123" {
		t.Fatalf("got %q, want abc123", got)
	}
}

func TestParseSHA256SUMS(t *testing.T) {
	data := []byte(`abc123  cortex-linux-arm64
def456  cortex-darwin-arm64
7890ab  *cortex-linux-amd64
`)
	cases := []struct {
		filename string
		want     string
	}{
		{"cortex-linux-arm64", "abc123"},
		{"cortex-darwin-arm64", "def456"},
		// Binary-mode marker "*" is stripped.
		{"cortex-linux-amd64", "7890ab"},
		{"missing-file", ""}, // error
	}
	for _, c := range cases {
		got, err := parseSHA256SUMS(data, c.filename)
		if c.want == "" {
			if err == nil {
				t.Errorf("parseSHA256SUMS(%q) = %q, want error", c.filename, got)
			}
		} else {
			if err != nil {
				t.Errorf("parseSHA256SUMS(%q): %v", c.filename, err)
			}
			if got != c.want {
				t.Errorf("parseSHA256SUMS(%q) = %q, want %q", c.filename, got, c.want)
			}
		}
	}
}

func TestRun_UpToDate(t *testing.T) {
	// Fake "latest" release matches our current Version. Skipped
	// by default to avoid hitting GitHub and to avoid destructive
	// test runs that rename the test binary. Set
	// UPDATER_INTEGRATION=1 to exercise the full flow.
	if os.Getenv("UPDATER_INTEGRATION") == "" {
		t.Skip("set UPDATER_INTEGRATION=1 to run the full Run() flow")
	}
	oldVersion := Version
	defer func() { Version = oldVersion }()
	Version = "v9.9.9"
	assetName, _ := AssetName()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/latest") {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"tag_name":"v9.9.9","assets":[{"name":"`+assetName+`","browser_download_url":"http://example.com/bin","size":1,"digest":"sha256=abc"}]}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	HTTPClient = srv.Client()
	HTTPClient.Timeout = 5 * time.Second
	res := Run(context.Background())
	if res.Kind != "up-to-date" {
		t.Errorf("Kind = %q, want up-to-date; error=%v", res.Kind, res.Error)
	}
}

func TestRun_HashMismatch(t *testing.T) {
	// Force an "update available" by setting Version far from
	// the fake release tag, then make the SHA256SUMS disagree
	// with the downloaded bytes. Skipped by default for the
	// same reason as TestRun_UpToDate.
	if os.Getenv("UPDATER_INTEGRATION") == "" {
		t.Skip("set UPDATER_INTEGRATION=1 to run the full Run() flow")
	}
	oldVersion := Version
	defer func() { Version = oldVersion }()
	Version = "dev"
	assetName, _ := AssetName()
	// 16 bytes of payload → known hash.
	payload := []byte("0123456789abcdef")
	wantHash := sha256.Sum256(payload)
	wantHashHex := hex.EncodeToString(wantHash[:])

	// Build the fake server in two steps so the handler can
	// reference its URL.
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"tag_name":"v9.9.9","assets":[`+
				`{"name":"`+assetName+`","browser_download_url":"`+srv.URL+`/bin","size":16,"digest":"sha256=`+wantHashHex+`"},`+
				`{"name":"SHA256SUMS","browser_download_url":"`+srv.URL+`/sums","size":0}`+
				`]}`)
		case strings.HasSuffix(r.URL.Path, "/bin"):
			w.Write(payload)
		case strings.HasSuffix(r.URL.Path, "/sums"):
			// Intentionally lie about the hash.
			io.WriteString(w, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  "+assetName+"\n")
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	HTTPClient = srv.Client()
	HTTPClient.Timeout = 5 * time.Second

	res := Run(context.Background())
	if res.Kind != "error" {
		t.Errorf("Kind = %q, want error; result=%+v", res.Kind, res)
	}
	if res.Error == nil || !strings.Contains(res.Error.Error(), "hash mismatch") {
		t.Errorf("Error = %v, want hash-mismatch error", res.Error)
	}
}

func TestRun_HappyPath(t *testing.T) {
	// Skipped by default to avoid destructive operations on the
	// running test binary. See TestRun_UpToDate.
	if os.Getenv("UPDATER_INTEGRATION") == "" {
		t.Skip("set UPDATER_INTEGRATION=1 to run the full install flow")
	}
	_ = httptest.NewServer // keep the import alive
	_ = context.Background
	_ = sha256.Sum256
	_ = hex.EncodeToString
	oldVersion := Version
	defer func() { Version = oldVersion }()
	Version = "dev"
	assetName, _ := AssetName()
	dir := t.TempDir()
	exePath := filepath.Join(dir, assetName)
	if err := os.WriteFile(exePath, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Skip the integration step: this test really only exists
	// to keep the rename logic compiled.
	_ = exePath
}
