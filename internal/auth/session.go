package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// MinSessionKeyLen is the minimum HMAC key length for session cookies.
const MinSessionKeyLen = 32

// Session is the payload of a UI session cookie.
type Session struct {
	User    string `json:"u"`
	Expires int64  `json:"e"` // unix seconds
}

// SessionCodec signs and verifies stateless session cookie values with
// HMAC-SHA256, so any UI replica sharing the key can validate a session.
type SessionCodec struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

// NewSessionCodec returns a codec issuing sessions valid for ttl.
func NewSessionCodec(key []byte, ttl time.Duration) (*SessionCodec, error) {
	if len(key) < MinSessionKeyLen {
		return nil, errors.New("session key must be at least 32 bytes")
	}
	if ttl <= 0 {
		return nil, errors.New("session ttl must be positive")
	}
	return &SessionCodec{key: append([]byte(nil), key...), ttl: ttl, now: time.Now}, nil
}

// TTL is how long issued sessions remain valid.
func (c *SessionCodec) TTL() time.Duration { return c.ttl }

// Issue returns a signed cookie value for user.
func (c *SessionCodec) Issue(user string) (string, error) {
	payload, err := json.Marshal(Session{User: user, Expires: c.now().Add(c.ttl).Unix()})
	if err != nil {
		return "", err
	}
	p := base64.RawURLEncoding.EncodeToString(payload)
	return p + "." + base64.RawURLEncoding.EncodeToString(c.sign(p)), nil
}

// Verify checks the signature and expiry of a cookie value.
func (c *SessionCodec) Verify(value string) (*Session, error) {
	p, sig, ok := strings.Cut(value, ".")
	if !ok {
		return nil, errors.New("malformed session")
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, c.sign(p)) {
		return nil, errors.New("invalid session signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return nil, errors.New("malformed session")
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil || s.User == "" {
		return nil, errors.New("malformed session")
	}
	if c.now().Unix() >= s.Expires {
		return nil, errors.New("session expired")
	}
	return &s, nil
}

func (c *SessionCodec) sign(payload string) []byte {
	m := hmac.New(sha256.New, c.key)
	m.Write([]byte("conduit-session-v1\x00"))
	m.Write([]byte(payload))
	return m.Sum(nil)
}
