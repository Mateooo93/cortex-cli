// Device-code OAuth flow for ChatGPT (codex) login.
//
// The browser flow (oauth.go) opens the user's default browser and
// waits for a callback on localhost:1455. That works on laptops but
// breaks in a few real-world situations:
//
//   - SSH / WSL / remote dev environments where the user can't open
//     a browser on the same machine.
//   - Cloud VMs / containers that don't have a default browser.
//   - OpenAI's auth server recently started requiring phone
//     verification for the localhost-callback flow on accounts
//     that don't have a phone number on file, which surfaces as
//     "Invalid authorize request" in the browser
//     (https://github.com/openai/codex/issues/20161).
//
// The device-code flow sidesteps both problems:
//
//  1. POST to https://auth.openai.com/deviceauth/usercode with the
//     client_id. The server returns a one-time user_code and a
//     device_auth_id.
//  2. The TUI prints the verification URL
//     (https://auth.openai.com/codex/device) and the user_code.
//     The user opens the URL in any browser on any device, signs
//     in, and pastes the code.
//  3. We poll https://auth.openai.com/deviceauth/token with the
//     device_auth_id + user_code. When the user completes the
//     browser-side auth, the server returns an authorization_code
//     + PKCE verifier pair (the server ran PKCE for us).
//  4. We exchange the code for tokens using the same codex token
//     endpoint as the browser flow.
//
// This matches the official `codex login --device-auth` flow
// (openai/codex, codex-rs/login/src/device_code_auth.rs).

package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DeviceAuthEndpoint is the base URL for the device-code endpoints.
// Declared as a var so tests can swap it for an httptest server.
var DeviceAuthEndpoint = "https://auth.openai.com/deviceauth"

// DeviceVerificationURL is the URL the user opens in their browser
// to enter the one-time code. It's hard-coded (not in the response)
// so we can show it as soon as we get the user_code back.
const DeviceVerificationURL = "https://auth.openai.com/codex/device"

// DeviceCode is the result of requesting a user code from the
// device-auth endpoint. The user opens VerificationURL in their
// browser and enters UserCode. The TUI then polls TokenEndpoint
// (in this package: TokenEndpoint + /deviceauth/token) until the
// user completes the flow or DeviceCode.ExpiresAt is reached.
type DeviceCode struct {
	UserCode        string    // 9-char code the user pastes in the browser
	DeviceAuthID    string    // opaque ID for the polling request
	Interval        time.Duration // suggested poll interval
	VerificationURL string    // the URL the user opens
	ExpiresAt       time.Time // when the code stops being valid (15 min)
}

// DeviceCodeResult is the final result of a device-code login,
// returned by DeviceLogin. It's the same shape as LoginResult so
// callers (model.go's handleCodexLoginSuccess etc.) don't have to
// branch on which flow the user took.
type DeviceCodeResult struct {
	*LoginResult

	// UserCode is included so the UI can echo it back after the
	// poll completes ("code XYZ-ABC-123 was redeemed at ...").
	// Most callers ignore this.
	UserCode string
}

// DeviceCodeRequest is the JSON body of the usercode POST.
type deviceCodeRequest struct {
	ClientID string `json:"client_id"`
}

// DeviceCodeResponse is the JSON body of the usercode POST response.
type deviceCodeResponse struct {
	DeviceAuthID string `json:"device_auth_id"`
	UserCode     string `json:"user_code"`
	// OpenAI returns the interval as a string ("5") not a number.
	Interval string `json:"interval"`
}

// DeviceTokenSuccess is the success body of the token-poll POST.
type deviceTokenSuccess struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeChallenge     string `json:"code_challenge"`
	CodeVerifier      string `json:"code_verifier"`
}

// DeviceTokenPending is the body when the user hasn't completed
// the browser-side auth yet.
type deviceTokenPending struct {
	Error string `json:"error"`
}

// RequestDeviceCode asks OpenAI's auth server for a one-time
// user_code. The returned DeviceCode carries everything the UI
// needs to show a "open this URL, enter this code" prompt plus
// the IDs needed to poll for completion.
func RequestDeviceCode(ctx context.Context) (*DeviceCode, error) {
	body, _ := json.Marshal(deviceCodeRequest{ClientID: ClientID})
	req, err := http.NewRequestWithContext(ctx, "POST",
		DeviceAuthEndpoint+"/usercode",
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("codex: devicecode request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("codex: devicecode: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("codex: devicecode: %s: %s", resp.Status, string(buf))
	}
	var out deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("codex: devicecode decode: %w", err)
	}
	if out.UserCode == "" || out.DeviceAuthID == "" {
		return nil, errors.New("codex: devicecode response missing user_code or device_auth_id")
	}
	// Parse the interval; default to 5s on parse failure
	// (matches the official CLI's default of 5).
	interval := 5 * time.Second
	if out.Interval != "" {
		var secs int
		if _, err := fmt.Sscanf(out.Interval, "%d", &secs); err == nil && secs > 0 {
			interval = time.Duration(secs) * time.Second
		}
	}
	return &DeviceCode{
		UserCode:        out.UserCode,
		DeviceAuthID:    out.DeviceAuthID,
		Interval:        interval,
		VerificationURL: DeviceVerificationURL,
		ExpiresAt:       time.Now().Add(15 * time.Minute),
	}, nil
}

// DeviceLogin runs the full device-code flow: request a user_code,
// poll for the user to complete the browser-side auth, then
// exchange the resulting authorization code for tokens.
//
// Polling continues until the user completes the flow, the
// 15-minute expiry passes, or ctx is cancelled. The interval
// between polls is taken from the server's response and doubled
// after each "pending" response (matches the official CLI's
// backoff behavior in codex-rs/login/src/device_code_auth.rs).
//
// DeviceLogin always returns a non-nil *DeviceCodeResult so the
// caller can surface the user_code in the success message even
// when the actual token exchange succeeds.
func DeviceLogin(ctx context.Context) (*DeviceCodeResult, error) {
	dc, err := RequestDeviceCode(ctx)
	if err != nil {
		return nil, err
	}
	authCode, codeVerifier, err := pollDeviceToken(ctx, dc)
	if err != nil {
		return nil, err
	}
	if authCode == "" {
		return nil, errors.New("codex: devicecode: empty authorization code returned")
	}
	// The server gave us the PKCE verifier directly, so the
	// exchange is a standard authorization_code grant with the
	// server's code_verifier (not one we generated).
	tok, err := exchangeCode(ctx, authCode, codeVerifier, "http://localhost:1455/auth/callback")
	if err != nil {
		return nil, fmt.Errorf("codex: devicecode token exchange: %w", err)
	}
	return &DeviceCodeResult{
		LoginResult: &LoginResult{
			Token:        tok,
			AuthorizeURL: dc.VerificationURL,
		},
		UserCode: dc.UserCode,
	}, nil
}

// pollDeviceToken posts to the device-auth token endpoint every
// dc.Interval seconds (with exponential backoff on pending
// responses) until the user completes the flow or the code
// expires. Returns the authorization_code + code_verifier on
// success.
func pollDeviceToken(ctx context.Context, dc *DeviceCode) (authCode, codeVerifier string, err error) {
	body, _ := json.Marshal(map[string]string{
		"device_auth_id": dc.DeviceAuthID,
		"user_code":      dc.UserCode,
	})
	interval := dc.Interval
	if interval < time.Second {
		interval = time.Second
	}
	for {
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(time.Until(dc.ExpiresAt)):
			if time.Until(dc.ExpiresAt) <= 0 {
				return "", "", errors.New("codex: devicecode: user code expired (you took longer than 15 minutes)")
			}
		default:
		}
		req, rerr := http.NewRequestWithContext(ctx, "POST",
			DeviceAuthEndpoint+"/token",
			bytes.NewReader(body))
		if rerr != nil {
			return "", "", rerr
		}
		req.Header.Set("Content-Type", "application/json")
		resp, rerr := http.DefaultClient.Do(req)
		if rerr != nil {
			return "", "", rerr
		}
		// 200 = success, 4xx with error=authorization_pending =
		// user hasn't completed the browser flow yet. Anything
		// else is a real error.
		if resp.StatusCode == http.StatusOK {
			var ok deviceTokenSuccess
			if derr := json.NewDecoder(resp.Body).Decode(&ok); derr != nil {
				resp.Body.Close()
				return "", "", derr
			}
			resp.Body.Close()
			return ok.AuthorizationCode, ok.CodeVerifier, nil
		}
		// Read the body for diagnostic purposes; on pending
		// responses it's just `{"error":"authorization_pending"}`.
		buf, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var pending deviceTokenPending
		if json.Unmarshal(buf, &pending) == nil && pending.Error == "authorization_pending" {
			// Backoff: double the interval up to 30s.
			interval *= 2
			if interval > 30*time.Second {
				interval = 30 * time.Second
			}
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-time.After(interval):
			}
			continue
		}
		return "", "", fmt.Errorf("codex: devicecode poll: %s: %s", resp.Status, string(buf))
	}
}
