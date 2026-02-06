package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

const tokenBytes = 32

// Token prefixes for identification.
const (
	PrefixTeamToken         = "mtk_"
	PrefixRunnerToken       = "mtr_"
	PrefixRegistrationToken = "mtreg_"
)

// GenerateToken returns a new random token and its SHA-256 hash.
func GenerateToken() (string, string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}

	raw := base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashToken(raw), nil
}

// GeneratePrefixedToken returns a new prefixed token and its SHA-256 hash.
func GeneratePrefixedToken(prefix string) (string, string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}

	raw := prefix + base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashToken(raw), nil
}

// HashToken returns a SHA-256 hex digest for a token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
