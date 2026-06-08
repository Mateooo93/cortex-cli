package xaisub

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

type jwtClaims struct {
	Email     string
	ExpiresAt time.Time
}

func parseJWT(jwt string) (jwtClaims, bool) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return jwtClaims{}, false
	}
	payload, err := base64.URLEncoding.DecodeString(padBase64(parts[1]))
	if err != nil {
		return jwtClaims{}, false
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return jwtClaims{}, false
	}
	c := jwtClaims{Email: stringClaim(raw, "email")}
	if exp, ok := numberClaim(raw, "exp"); ok {
		c.ExpiresAt = time.Unix(exp, 0)
	}
	if c.Email == "" && c.ExpiresAt.IsZero() {
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

func padBase64(s string) string {
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	return s
}