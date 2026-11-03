package sdk

import (
	"crypto/rand"
	"encoding/hex"
)

// newTraceID returns a random 16-byte (32 hex char) trace ID and newSpanID a
// random 8-byte (16 hex char) span ID, the sizes W3C Trace Context and OTLP
// require. Tempo rejects spans whose IDs are any other length.
func newTraceID() string { return randomHex(16) }
func newSpanID() string  { return randomHex(8) }

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
