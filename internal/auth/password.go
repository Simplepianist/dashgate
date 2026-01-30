package auth

import (
	"crypto/sha256"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a password using SHA-256 + bcrypt to handle passwords
// longer than bcrypt's 72-byte limit. The password is first hashed with SHA-256
// to produce a fixed-length input, then that hash is passed to bcrypt.
func HashPassword(password string) (string, error) {
	sha := sha256.Sum256([]byte(password))
	preHashed := hex.EncodeToString(sha[:])
	hash, err := bcrypt.GenerateFromPassword([]byte(preHashed), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword verifies a password against a bcrypt hash.
// It first tries the new SHA-256 pre-hash method, then falls back to
// legacy direct bcrypt comparison for backward compatibility with
// existing password hashes created before the pre-hash was introduced.
func CheckPassword(password, hash string) bool {
	// Try new SHA-256 pre-hash method first
	sha := sha256.Sum256([]byte(password))
	preHashed := hex.EncodeToString(sha[:])
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(preHashed)) == nil {
		return true
	}
	// Fall back to legacy direct bcrypt for existing hashes
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
