package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// RandomToken returns nBytes of crypto/rand entropy encoded as a URL-safe
// base64 string. It backs session cookies, CSRF tokens, and node tokens.
func RandomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
