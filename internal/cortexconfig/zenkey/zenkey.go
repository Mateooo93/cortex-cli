package zenkey

import (
	"crypto/aes"
	"crypto/cipher"
	"sync"
)

var (
	cached     string
	cachedOnce sync.Once
)

// Key returns the bundled OpenCode Zen API key, or empty if unavailable.
func Key() string {
	cachedOnce.Do(func() {
		cached = decrypt()
	})
	return cached
}

// Available reports whether a bundled Zen key is embedded in this binary.
func Available() bool {
	return len(encCipher) > 0 && len(encNonce) == 12
}

func decrypt() string {
	if !Available() {
		return ""
	}
	block, err := aes.NewCipher(deriveKey())
	if err != nil {
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ""
	}
	plain, err := gcm.Open(nil, encNonce[:], encCipher, nil)
	if err != nil {
		return ""
	}
	return string(plain)
}
