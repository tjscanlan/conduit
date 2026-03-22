package sdk

import (
	"crypto/rand"
	"encoding/hex"
)

// newID generates a cryptographically random 32-hex-char ID suitable for trace/span IDs.
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
