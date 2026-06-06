// Package codex implements ChatGPT OAuth (PKCE) login for cortex-cli.
//
// The flow:
//
//  1. Generate a PKCE verifier+challenge and a CSRF state token.
//  2. Spin up a local HTTP server on the loopback callback port (default
//     1455, matching the official Codex CLI). The handler validates the
//     state, captures the ?code=…, and renders a "you can close this tab"
//     page.
//  3. Open the user's default browser at the authorize URL. They sign in
//     to ChatGPT, approve the device, and the browser is redirected back
//     to our local server.
//  4. Exchange the code for an access + refresh token bundle against
//     auth.openai.com's /token endpoint, signed with the PKCE verifier.
//  5. Return the bundle. Callers store it in the OS keychain.
//
// The access token is a JWT; the chatgpt-account-id, plan type, and exp
// are extracted in auth.go.
package codex

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
	"time"
)

const (
	// AuthEndpoint is OpenAI's OAuth authorize URL for ChatGPT logins.
	AuthEndpoint = "https://auth.openai.com/oauth/authorize"
	// ClientID is the public OAuth client_id used by the official
	// Codex CLI (verified against openai/codex, codex-rs/login).
	// It is intentionally a public client — no secret. The value
	// `app_EMoamEEZ73f0CkXaXp7hrann` is the only one OpenAI's
	// authorize endpoint accepts for this flow; passing any other
	// (e.g. the older `app_oauth_agent`) returns `invalid_client`.
	ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	// DefaultCallbackPort matches the port the official Codex CLI uses.
	// The same number is hard-coded in the authorize URL the user clicks,
	// so changing it would break the redirect.
	DefaultCallbackPort = 1455
	// scope is the OpenID scope set the official Codex CLI requests
	// (verified against openai/codex, codex-rs/login/src/server.rs).
	// The old "com.chatgpt.agent.completion" scope has been replaced
	// by the connector scopes which is what the ChatGPT backend
	// expects as of mid-2026.
	scope = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	// originator is a query param OpenAI's auth server uses to
	// identify which client is initiating the flow. The Codex CLI
	// sends `codex_cli_rs`; sending a missing or unrecognized
	// value triggers the "Invalid client specified" error in the
	// token-exchange step.
	originator = "codex_cli_rs"
)

// TokenEndpoint is OpenAI's OAuth token-exchange URL. Declared as a
// var so tests can swap it for an httptest server.
var TokenEndpoint = "https://auth.openai.com/oauth/token"

// Token is the persisted bundle from a successful OAuth exchange.
// Access + Refresh are both stored so we can refresh transparently on
// 401. AccountID and Email are extracted from the JWT for use as
// request headers / display purposes.
type Token struct {
	Access    string    `json:"access_token"`
	Refresh   string    `json:"refresh_token,omitempty"`
	ID        string    `json:"id_token,omitempty"`
	AccountID string    `json:"account_id,omitempty"`
	Email     string    `json:"email,omitempty"`
	PlanType  string    `json:"plan_type,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	Scope     string    `json:"scope,omitempty"`
}

// Expired reports whether the access token is past its exp.
func (t *Token) Expired() bool {
	if t.ExpiresAt.IsZero() {
		return true
	}
	// Treat as expired 60s early to avoid races.
	return time.Now().Add(60 * time.Second).After(t.ExpiresAt)
}

// LoginResult bundles the resulting token with the URL the user was
// told to visit, so callers can fall back to displaying the URL when
// the browser couldn't be opened automatically.
type LoginResult struct {
	Token       *Token
	AuthorizeURL string
	Port         int
}

// Login starts a PKCE OAuth login flow, opens the user's browser, and
// blocks until the local callback server either captures the code or
// the context is cancelled. Pass ctx to add an overall deadline.
func Login(ctx context.Context) (*LoginResult, error) {
	verifier, challenge, err := pkce()
	if err != nil {
		return nil, fmt.Errorf("codex: pkce: %w", err)
	}
	state, err := randB64URL(32)
	if err != nil {
		return nil, fmt.Errorf("codex: state: %w", err)
	}

	port, err := pickPort(DefaultCallbackPort)
	if err != nil {
		return nil, fmt.Errorf("codex: callback port: %w", err)
	}
	redirectURI := fmt.Sprintf("http://localhost:%d/auth/callback", port)
	authURL := buildAuthURL(redirectURI, state, challenge)

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

	// Give the server a moment to bind.
	select {
	case <-waitForReady(port, 1*time.Second):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Best-effort browser launch. We don't fail the login if xdg-open
	// isn't there — the user can copy the URL from the TUI.
	browserOpened := openBrowser(authURL)

	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, fmt.Errorf("codex: callback: %w", res.err)
		}
		tok, err := exchangeCode(ctx, res.code, verifier, redirectURI)
		if err != nil {
			return nil, err
		}
		return &LoginResult{Token: tok, AuthorizeURL: authURL, Port: port}, browserOpenedErr(browserOpened)
	case <-ctx.Done():
		return &LoginResult{AuthorizeURL: authURL, Port: port}, ctx.Err()
	}
}

// Refresh trades a refresh_token for a fresh access token. Returns the
// new Token bundle (with a new refresh_token when the server provides
// one). Callers should replace their stored Token with the returned one.
func Refresh(ctx context.Context, refreshToken string) (*Token, error) {
	if refreshToken == "" {
		return nil, errors.New("codex: empty refresh token")
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {ClientID},
		"refresh_token": {refreshToken},
	}
	req, err := http.NewRequestWithContext(ctx, "POST", TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("codex: refresh: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("codex: refresh: %d %s", resp.StatusCode, truncate(string(body), 200))
	}
	tok, err := parseTokenResponse(body)
	if err != nil {
		return nil, err
	}
	return tok, nil
}

// buildAuthURL assembles the authorize URL with all PKCE params.
// Matches the openai/codex CLI flow (codex-rs/login/src/server.rs).
func buildAuthURL(redirectURI, state, challenge string) string {
	q := url.Values{
		"client_id":                    {ClientID},
		"response_type":                {"code"},
		"redirect_uri":                 {redirectURI},
		"scope":                        {scope},
		"state":                        {state},
		"code_challenge":               {challenge},
		"code_challenge_method":        {"S256"},
		"id_token_add_organizations":   {"true"},
		"codex_cli_simplified_flow":    {"true"},
		"originator":                   {originator},
	}
	return AuthEndpoint + "?" + q.Encode()
}

// AuthURL builds a fresh authorize URL with a new state and
// PKCE verifier. The UI uses this to display the URL up front
// (in the "waiting for auth" overlay) so the user can copy it
// into a browser manually if the auto-open fails. The URL is
// not associated with a specific Login() call — the user has
// to start Login() separately, and the verifier + state in
// that flow will be different. The two flows are
// independent: the on-screen URL is for "fallback / manual"
// sign-in; the auto-flow runs in parallel and updates the
// TUI when the browser callback arrives.
func AuthURL() string {
	_, challenge, _ := pkce()
	state, _ := randB64URL(16)
	redirectURI := fmt.Sprintf("http://localhost:%d/auth/callback", DefaultCallbackPort)
	return buildAuthURL(redirectURI, state, challenge)
}

// pkce returns a base64url(verifier) and base64url(sha256(verifier)).
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

// pickPort tries preferred first; if it's busy, falls back to a random
// free port. We bind on 127.0.0.1 only. Returns the actual port the
// listener was bound to (important when the caller passes 0, meaning
// "any free port").
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

// waitForReady polls the callback server until it accepts connections or
// the timeout elapses.
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
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if errParam := q.Get("error"); errParam != "" {
			writeCallbackPage(w, "Sign-in failed", errParam)
			out <- callbackResult{err: fmt.Errorf("oauth error: %s", errParam)}
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
	// Friendly root: shows a hint instead of a 404 if the user navigates manually.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>cortex-cli</title>
<body style="font-family:system-ui;max-width:480px;margin:80px auto;text-align:center">
<h2>cortex-cli</h2>
<p>Waiting for sign-in&hellip; complete the ChatGPT page in the popup and this window will close automatically.</p>
</body>`)
	})
	return mux
}

func writeCallbackPage(w http.ResponseWriter, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>cortex-cli: %s</title>
<body style="font-family:system-ui;max-width:520px;margin:80px auto;text-align:center;color:#222">
<h2 style="color:#3D8BFF">%s</h2>
<p>%s</p>
<p style="margin-top:32px;color:#888;font-size:13px">You can close this tab and return to cortex-cli.</p>
</body>`, title, title, body)
}

// exchangeCode trades an authorization code + PKCE verifier for tokens.
func exchangeCode(ctx context.Context, code, verifier, redirectURI string) (*Token, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {ClientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, "POST", TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("codex: exchange: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("codex: exchange: %d %s", resp.StatusCode, truncate(string(body), 200))
	}
	return parseTokenResponse(body)
}

// tokenResponse is the OAuth token JSON shape. We only need the subset
// cortex-cli uses; the rest is dropped on parse.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

func parseTokenResponse(body []byte) (*Token, error) {
	var r tokenResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("codex: parse token: %w", err)
	}
	if r.AccessToken == "" {
		return nil, errors.New("codex: empty access_token in response")
	}
	t := &Token{
		Access:    r.AccessToken,
		Refresh:   r.RefreshToken,
		ID:        r.IDToken,
		ExpiresAt: time.Now().Add(time.Duration(r.ExpiresIn) * time.Second),
		Scope:     r.Scope,
	}
	// The id_token (preferred) or access_token is a JWT carrying the
	// chatgpt-account-id and email claims. Parse whichever we have.
	jwt := r.IDToken
	if jwt == "" {
		jwt = r.AccessToken
	}
	if claims, ok := parseJWT(jwt); ok {
		t.AccountID = claims.AccountID
		t.Email = claims.Email
		t.PlanType = claims.PlanType
		// The JWT's own exp is more authoritative than expires_in.
		if !claims.ExpiresAt.IsZero() {
			t.ExpiresAt = claims.ExpiresAt
		}
	}
	return t, nil
}

// openBrowser launches the system default browser with url. Returns
// an error if the launch command isn't available, but never blocks.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		// Linux / *BSD: try xdg-open, then sensible-browser, then wslview.
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

// browserOpenedErr turns the openBrowser result into a meaningful error
// (or nil). Surfaced so callers can decide whether to print the URL.
func browserOpenedErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("codex: open browser: %w (open the URL above manually)", err)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
