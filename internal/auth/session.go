package auth

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateSessionToken creates a cryptographically random 32-byte session
// token and returns it as a hex-encoded string.
func GenerateSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
