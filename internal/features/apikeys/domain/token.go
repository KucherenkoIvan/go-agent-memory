package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// TokenPrefix marks every agmem API key; clients validate it before dialing.
const TokenPrefix = "agm_"

const displayPrefixLength = 12

// GenerateToken mints a raw API key plus what gets stored: its hash and a
// short display prefix. The raw token exists only in the creation response.
func GenerateToken() (raw, hash, prefix string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("generating api key: %w", err)
	}
	raw = TokenPrefix + hex.EncodeToString(buf)
	return raw, HashToken(raw), raw[:displayPrefixLength], nil
}

// HashToken is how tokens are stored and looked up — sha256 hex, never raw.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// LooksLikeToken is the cheap client-side sanity check.
func LooksLikeToken(raw string) bool {
	return strings.HasPrefix(raw, TokenPrefix) && len(raw) > displayPrefixLength
}
