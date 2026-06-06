package codex

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// jwtClaims is the subset of the ChatGPT access/id_token claims cortex-cli
// uses. The actual JWT carries a lot more, but we only need:
//
//   • https://api.openai.com/auth.chatgpt_account_id — required as the
//     `chatgpt-account-id` header on API calls.
//   • https://api.openai.com/auth.chatgpt_plan_type  — for the Settings
//     tab ("You are signed in as Plus/Pro/Team…").
//   • email — for the same.
//   • exp   — server-authoritative expiry (we also accept expires_in).
//
// The custom `https://api.openai.com/auth` claim lives in the "Auth"
// field below; its key names use dots which are awkward in Go, so we
// keep them as nested maps and pluck what we need at parse time.
type jwtClaims struct {
	Email     string
	AccountID string
	PlanType  string
	ExpiresAt time.Time
}

// parseJWT extracts the cortex-relevant claims from a JWT body. Returns
// ok=false if the input isn't a recognisable JWT (caller should fall
// back to the expires_in in the token-response JSON).
func parseJWT(jwt string) (jwtClaims, bool) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return jwtClaims{}, false
	}
	// URLEncoding (not RawURLEncoding) is the right decoder for
	// padded base64url. JWT segments drop the = padding, so we
	// restore it before decoding.
	payload, err := base64.URLEncoding.DecodeString(padBase64(parts[1]))
	if err != nil {
		return jwtClaims{}, false
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return jwtClaims{}, false
	}
	c := jwtClaims{
		Email: stringClaim(raw, "email"),
	}
	// exp / iat are top-level unix seconds.
	if exp, ok := numberClaim(raw, "exp"); ok {
		c.ExpiresAt = time.Unix(exp, 0)
	}
	// Custom OpenAI claims live under https://api.openai.com/auth.
	if auth, ok := raw["https://api.openai.com/auth"].(map[string]any); ok {
		c.AccountID = stringClaim(auth, "chatgpt_account_id")
		c.PlanType = stringClaim(auth, "chatgpt_plan_type")
		// Some token shapes nest one more level ("user" / "org"). Try a
		// few common keys so we don't fail on minor schema differences.
		if c.AccountID == "" {
			if user, ok := auth["user"].(map[string]any); ok {
				c.AccountID = stringClaim(user, "id")
			}
		}
	}
	// Anything missing? Treat the parse as failed so caller falls back.
	if c.AccountID == "" && c.Email == "" && c.ExpiresAt.IsZero() {
		return jwtClaims{}, false
	}
	return c, true
}

func stringClaim(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func numberClaim(m map[string]any, key string) (int64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}

// padBase64 adds the missing = padding that JWT base64url segments
// drop. Go's URLEncoding requires the padding to be present, so we
// re-add it here.
func padBase64(s string) string {
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	return s
}

// String formats a Token for log/status display, redacting secrets.
func (t *Token) String() string {
	if t == nil {
		return "<nil codex.Token>"
	}
	return fmt.Sprintf("codex.Token{email=%q account=%q plan=%q exp=%s}",
		t.Email, mask(t.AccountID, 8), t.PlanType, t.ExpiresAt.Format(time.RFC3339))
}

// mask returns the last n chars of s with the rest replaced by •, so
// logs / status output never spill a full account id.
func mask(s string, n int) string {
	if s == "" {
		return ""
	}
	if len(s) <= n {
		return strings.Repeat("•", len(s))
	}
	return strings.Repeat("•", len(s)-n) + s[len(s)-n:]
}
