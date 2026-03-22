package auth

import (
	"crypto/subtle"
	"errors"
	"sync"
)

// TokenStore is an in-memory bearer token store for UI authentication.
// In production, replace with a database-backed or secrets-manager-backed store.
type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]string // token → label (e.g. "ui-admin")
}

func NewTokenStore() *TokenStore {
	return &TokenStore{tokens: make(map[string]string)}
}

// Add registers a token with an associated label.
func (t *TokenStore) Add(token, label string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tokens[token] = label
}

// Validate performs a constant-time comparison against all stored tokens.
// Returns the label on success, or an error if no match is found.
func (t *TokenStore) Validate(token string) (string, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for stored, label := range t.tokens {
		if subtle.ConstantTimeCompare([]byte(token), []byte(stored)) == 1 {
			return label, nil
		}
	}
	return "", errors.New("invalid token")
}
