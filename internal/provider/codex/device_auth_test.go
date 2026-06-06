package codex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRequestDeviceCode_HappyPath uses an httptest server to fake
// the usercode endpoint and verifies that the parsed DeviceCode
// carries the expected fields. Catches URL/JSON drift if OpenAI
// ever changes the response shape.
func TestRequestDeviceCode_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/usercode" {
			t.Errorf("path = %q, want /usercode", r.URL.Path)
		}
		var body deviceCodeRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.ClientID != ClientID {
			t.Errorf("client_id = %q, want %q", body.ClientID, ClientID)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"device_auth_id":"dev-abc","user_code":"ABCD-1234","interval":"5"}`)
	}))
	defer srv.Close()
	oldBase := DeviceAuthEndpoint
	DeviceAuthEndpoint = srv.URL
	defer func() { DeviceAuthEndpoint = oldBase }()

	dc, err := RequestDeviceCode(context.Background())
	if err != nil {
		t.Fatalf("RequestDeviceCode: %v", err)
	}
	if dc.UserCode != "ABCD-1234" {
		t.Errorf("UserCode = %q, want ABCD-1234", dc.UserCode)
	}
	if dc.DeviceAuthID != "dev-abc" {
		t.Errorf("DeviceAuthID = %q, want dev-abc", dc.DeviceAuthID)
	}
	if dc.Interval != 5*time.Second {
		t.Errorf("Interval = %v, want 5s", dc.Interval)
	}
	if dc.VerificationURL != DeviceVerificationURL {
		t.Errorf("VerificationURL = %q, want %q", dc.VerificationURL, DeviceVerificationURL)
	}
	if time.Until(dc.ExpiresAt) < 14*time.Minute {
		t.Errorf("ExpiresAt too close: %v", dc.ExpiresAt)
	}
}

// TestRequestDeviceCode_ServerError pins the failure mode: a non-2xx
// response from the usercode endpoint must surface a non-nil error
// so the TUI can show "device-code login failed: …" instead of
// silently showing an empty user_code.
func TestRequestDeviceCode_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		io.WriteString(w, `{"error":"internal_error"}`)
	}))
	defer srv.Close()
	oldBase := DeviceAuthEndpoint
	DeviceAuthEndpoint = srv.URL
	defer func() { DeviceAuthEndpoint = oldBase }()

	_, err := RequestDeviceCode(context.Background())
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want to contain 500", err.Error())
	}
}

// TestPollDeviceToken_AuthorizationPending verifies that the
// pending-response handler backs off and retries. The fake server
// returns pending twice then success; the test asserts the final
// authCode/codeVerifier are the ones in the success body.
func TestPollDeviceToken_AuthorizationPending(t *testing.T) {
	var pendingCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			t.Errorf("path = %q, want /token", r.URL.Path)
		}
		pendingCount++
		if pendingCount < 3 {
			w.WriteHeader(400)
			io.WriteString(w, `{"error":"authorization_pending"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"authorization_code":"auth-xyz","code_challenge":"chal","code_verifier":"ver"}`)
	}))
	defer srv.Close()
	oldBase := DeviceAuthEndpoint
	DeviceAuthEndpoint = srv.URL
	defer func() { DeviceAuthEndpoint = oldBase }()

	dc := &DeviceCode{
		UserCode:        "ABCD-1234",
		DeviceAuthID:    "dev-abc",
		Interval:        10 * time.Millisecond,
		VerificationURL: DeviceVerificationURL,
		ExpiresAt:       time.Now().Add(5 * time.Minute),
	}
	auth, ver, err := pollDeviceToken(context.Background(), dc)
	if err != nil {
		t.Fatalf("pollDeviceToken: %v", err)
	}
	if auth != "auth-xyz" {
		t.Errorf("authCode = %q, want auth-xyz", auth)
	}
	if ver != "ver" {
		t.Errorf("codeVerifier = %q, want ver", ver)
	}
	if pendingCount != 3 {
		t.Errorf("server hit %d times, want 3 (2 pending + 1 success)", pendingCount)
	}
}

// TestPollDeviceToken_Expired pins the behavior when the user takes
// longer than 15 minutes to sign in. We craft a DeviceCode that
// already expired and verify pollDeviceToken returns an error
// mentioning the expiry.
func TestPollDeviceToken_Expired(t *testing.T) {
	dc := &DeviceCode{
		UserCode:     "ABCD-1234",
		DeviceAuthID: "dev-abc",
		Interval:     10 * time.Millisecond,
		ExpiresAt:    time.Now().Add(-1 * time.Minute), // already expired
	}
	_, _, err := pollDeviceToken(context.Background(), dc)
	if err == nil {
		t.Fatal("expected expiry error, got nil")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error = %q, want to mention expiry", err.Error())
	}
}
