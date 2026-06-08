// Package xaisub implements xAI Grok OAuth (PKCE) login for cortex-cli.
//
// The flow matches Grok Build / the official Grok CLI:
//
//  1. Discover authorization + token endpoints from auth.x.ai.
//  2. Generate PKCE verifier/challenge and a CSRF state token.
//  3. Spin up a loopback HTTP server on 127.0.0.1:56121/callback.
//  4. Open the user's browser at accounts.x.ai to sign in.
//  5. Exchange the authorization code for access + refresh tokens.
//  6. Store the bundle in the OS keychain.
package xaisub

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	callbackPath = "/callback"
	// refreshSkew expires tokens 2 minutes early to avoid races.
	refreshSkew = 2 * time.Minute
)

// DiscoveryURL is xAI's OpenID configuration endpoint. Declared as a
// var so tests can point it at an httptest server.
var DiscoveryURL = "https://auth.x.ai/.well-known/openid-configuration"

const (
	// ClientID is the public OAuth client used by Grok CLI / Grok Build.
	ClientID = "b1a00492-073a-47ea-816f-4c329264a828"
	// Scope requests Grok CLI + API access for SuperGrok / X Premium+ accounts.
	Scope = "openid profile email offline_access grok-cli:access api:access"
	// DefaultCallbackPort matches the official Grok CLI loopback port.
	DefaultCallbackPort = 56121
)

// discovery holds the OAuth endpoints fetched from xAI.
type discovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

var (
	discoveryMu sync.Mutex
	discoveryCache *discovery
)

// Token is the persisted bundle from a successful OAuth exchange.
type Token struct {
	Access        string    `json:"access_token"`
	Refresh       string    `json:"refresh_token,omitempty"`
	ID            string    `json:"id_token,omitempty"`
	Email         string    `json:"email,omitempty"`
	ExpiresAt     time.Time `json:"expires_at"`
	Scope         string    `json:"scope,omitempty"`
	TokenEndpoint string    `json:"token_endpoint,omitempty"`
}

// Expired reports whether the access token is past its exp.
func (t *Token) Expired() bool {
	if t == nil || t.ExpiresAt.IsZero() {
		return true
	}
	return time.Now().Add(refreshSkew).After(t.ExpiresAt)
}

// LoginResult bundles the resulting token with the authorize URL.
type LoginResult struct {
	Token        *Token
	AuthorizeURL string
	Port         int
}

// Discover fetches (and caches) xAI's OAuth endpoints.
func Discover(ctx context.Context) (*discovery, error) {
	discoveryMu.Lock()
	if discoveryCache != nil {
		cached := *discoveryCache
		discoveryMu.Unlock()
		return &cached, nil
	}
	discoveryMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, DiscoveryURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xaisub: discovery: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xaisub: discovery: %d %s", resp.StatusCode, truncate(string(body), 200))
	}
	var d discovery
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("xaisub: discovery parse: %w", err)
	}
	if err := validateEndpoint(d.AuthorizationEndpoint); err != nil {
		return nil, err
	}
	if err := validateEndpoint(d.TokenEndpoint); err != nil {
		return nil, err
	}

	discoveryMu.Lock()
	discoveryCache = &d
	discoveryMu.Unlock()
	return &d, nil
}

func validateEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("xaisub: invalid endpoint %q: %w", raw, err)
	}
	host := strings.ToLower(u.Hostname())
	if u.Scheme != "https" || (host != "x.ai" && !strings.HasSuffix(host, ".x.ai")) {
		return fmt.Errorf("xaisub: unexpected OAuth endpoint: %s", raw)
	}
	return nil
}

// Login starts the PKCE OAuth flow, opens the browser, and blocks until
// the callback arrives or ctx is cancelled.
func Login(ctx context.Context) (*LoginResult, error) {
	d, err := Discover(ctx)
	if err != nil {
		return nil, err
	}

	verifier, challenge, err := pkce()
	if err != nil {
		return nil, fmt.Errorf("xaisub: pkce: %w", err)
	}
	state, err := randB64URL(32)
	if err != nil {
		return nil, fmt.Errorf("xaisub: state: %w", err)
	}
	nonce, err := randB64URL(32)
	if err != nil {
		return nil, fmt.Errorf("xaisub: nonce: %w", err)
	}

	port, err := pickPort(DefaultCallbackPort)
	if err != nil {
		return nil, fmt.Errorf("xaisub: callback port: %w", err)
	}
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", port, callbackPath)
	authURL := buildAuthURL(d.AuthorizationEndpoint, redirectURI, state, challenge, nonce)

	resCh := make(chan callbackResult, 1)
	srv := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: callbackHandler(state, resCh),
	}
	go func() { _ = srv.ListenAndServe() }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	select {
	case <-waitForReady(port, 1*time.Second):
	case <-ctx.Done():
		return &LoginResult{AuthorizeURL: authURL, Port: port}, ctx.Err()
	}

	browserOpened := openBrowser(authURL)

	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, fmt.Errorf("xaisub: callback: %w", res.err)
		}
		tok, err := exchangeCode(ctx, d.TokenEndpoint, res.code, verifier, redirectURI)
		if err != nil {
			return nil, err
		}
		return &LoginResult{Token: tok, AuthorizeURL: authURL, Port: port}, browserOpenedErr(browserOpened)
	case <-ctx.Done():
		return &LoginResult{AuthorizeURL: authURL, Port: port}, ctx.Err()
	}
}

// Refresh trades a refresh_token for a fresh access token.
func Refresh(ctx context.Context, tokenEndpoint, refreshToken string) (*Token, error) {
	if refreshToken == "" {
		return nil, errors.New("xaisub: empty refresh token")
	}
	if tokenEndpoint == "" {
		d, err := Discover(ctx)
		if err != nil {
			return nil, err
		}
		tokenEndpoint = d.TokenEndpoint
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {ClientID},
		"refresh_token": {refreshToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xaisub: refresh: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xaisub: refresh: %d %s", resp.StatusCode, truncate(string(body), 200))
	}
	tok, err := parseTokenResponse(body, tokenEndpoint)
	if err != nil {
		return nil, err
	}
	return tok, nil
}

// AuthURL builds an authorize URL for the waiting overlay. The URL is
// independent of any in-flight Login() call (same pattern as codex).
func AuthURL(ctx context.Context) (string, error) {
	d, err := Discover(ctx)
	if err != nil {
		return "", err
	}
	_, challenge, _ := pkce()
	state, _ := randB64URL(16)
	nonce, _ := randB64URL(16)
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", DefaultCallbackPort, callbackPath)
	return buildAuthURL(d.AuthorizationEndpoint, redirectURI, state, challenge, nonce), nil
}

func buildAuthURL(authEndpoint, redirectURI, state, challenge, nonce string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {Scope},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"nonce":                 {nonce},
	}
	return authEndpoint + "?" + q.Encode()
}

func pkce() (verifier, challenge string, err error) {
	v, err := randB64URL(64)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(v))
	c := base64.RawURLEncoding.EncodeToString(sum[:])
	return v, c, nil
}

func randB64URL(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func pickPort(preferred int) (int, error) {
	if preferred != 0 {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferred))
		if err == nil {
			port := l.Addr().(*net.TCPAddr).Port
			_ = l.Close()
			return port, nil
		}
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func waitForReady(port int, timeout time.Duration) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
			if err == nil {
				_ = c.Close()
				close(ch)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	return ch
}

type callbackResult struct {
	code string
	err  error
}

func callbackHandler(expectedState string, out chan<- callbackResult) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		writeCallbackCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		q := r.URL.Query()
		if errParam := q.Get("error"); errParam != "" {
			desc := q.Get("error_description")
			if desc == "" {
				desc = errParam
			}
			writeCallbackPage(w, "Sign-in failed", desc)
			out <- callbackResult{err: fmt.Errorf("oauth error: %s", desc)}
			return
		}
		if got := q.Get("state"); got != expectedState {
			writeCallbackPage(w, "Sign-in failed", "state mismatch (possible CSRF)")
			out <- callbackResult{err: errors.New("state mismatch")}
			return
		}
		code := q.Get("code")
		if code == "" {
			writeCallbackPage(w, "Sign-in failed", "no code in callback")
			out <- callbackResult{err: errors.New("missing code")}
			return
		}
		writeCallbackPage(w, "Signed in", "You can close this tab and return to cortex-cli.")
		out <- callbackResult{code: code}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>cortex-cli</title>
<body style="font-family:system-ui;max-width:480px;margin:80px auto;text-align:center">
<h2>cortex-cli</h2>
<p>Waiting for xAI sign-in&hellip; complete the Grok page in the popup and this window will close automatically.</p>
</body>`)
	})
	return mux
}

func writeCallbackCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "https://accounts.x.ai" || origin == "https://auth.x.ai" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		w.Header().Set("Vary", "Origin")
	}
}

func writeCallbackPage(w http.ResponseWriter, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>cortex-cli: %s</title>
<body style="font-family:system-ui;max-width:520px;margin:80px auto;text-align:center;color:#222">
<h2 style="color:#111">%s</h2>
<p>%s</p>
<p style="margin-top:32px;color:#888;font-size:13px">You can close this tab and return to cortex-cli.</p>
</body>`, title, title, body)
}

func exchangeCode(ctx context.Context, tokenEndpoint, code, verifier, redirectURI string) (*Token, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {ClientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xaisub: exchange: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xaisub: exchange: %d %s", resp.StatusCode, truncate(string(body), 200))
	}
	return parseTokenResponse(body, tokenEndpoint)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

func parseTokenResponse(body []byte, tokenEndpoint string) (*Token, error) {
	var r tokenResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("xaisub: parse token: %w", err)
	}
	if r.AccessToken == "" {
		return nil, errors.New("xaisub: empty access_token in response")
	}
	expiresIn := r.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	t := &Token{
		Access:        r.AccessToken,
		Refresh:       r.RefreshToken,
		ID:            r.IDToken,
		ExpiresAt:     time.Now().Add(time.Duration(expiresIn)*time.Second - refreshSkew),
		Scope:         r.Scope,
		TokenEndpoint: tokenEndpoint,
	}
	jwt := r.IDToken
	if jwt == "" {
		jwt = r.AccessToken
	}
	if claims, ok := parseJWT(jwt); ok {
		if claims.Email != "" {
			t.Email = claims.Email
		}
		if !claims.ExpiresAt.IsZero() {
			t.ExpiresAt = claims.ExpiresAt.Add(-refreshSkew)
		}
	}
	return t, nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		for _, name := range []string{"xdg-open", "sensible-browser", "wslview"} {
			if _, err := exec.LookPath(name); err == nil {
				cmd = exec.Command(name, url)
				break
			}
		}
	}
	if cmd == nil {
		return errors.New("no browser launcher found")
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func browserOpenedErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("xaisub: open browser: %w (open the URL above manually)", err)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}