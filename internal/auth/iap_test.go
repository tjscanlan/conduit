package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const aud = "/projects/123/global/backendServices/456"

type iapFixture struct {
	key     *ecdsa.PrivateKey
	kid     string
	fetches atomic.Int32
	v       *IAPVerifier
	now     time.Time
}

func newIAPFixture(t *testing.T) *iapFixture {
	t.Helper()
	f := &iapFixture{kid: "k1", now: time.Unix(1_800_000_000, 0)}
	f.key, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	b64 := func(i *big.Int) string { return base64.RawURLEncoding.EncodeToString(i.FillBytes(make([]byte, 32))) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.fetches.Add(1)
		fmt.Fprintf(w, `{"keys":[{"kty":"EC","crv":"P-256","kid":%q,"x":%q,"y":%q}]}`,
			f.kid, b64(f.key.PublicKey.X), b64(f.key.PublicKey.Y))
	}))
	t.Cleanup(srv.Close)
	f.v = &IAPVerifier{Audience: aud, KeysURL: srv.URL, Client: srv.Client(), Now: func() time.Time { return f.now }}
	return f
}

func (f *iapFixture) claims() map[string]any {
	return map[string]any{
		"iss": iapIssuer, "aud": aud, "sub": "accounts.google.com:1",
		"email": "bob@example.com", "hd": "example.com",
		"iat": f.now.Unix(), "exp": f.now.Add(10 * time.Minute).Unix(),
	}
}

func sign(t *testing.T, key *ecdsa.PrivateKey, header, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	input := enc(header) + "." + enc(claims)
	d := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, key, d[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (f *iapFixture) token(t *testing.T, mutate func(map[string]any)) string {
	c := f.claims()
	if mutate != nil {
		mutate(c)
	}
	return sign(t, f.key, map[string]any{"alg": "ES256", "kid": f.kid}, c)
}

func TestIAPVerifyValid(t *testing.T) {
	f := newIAPFixture(t)
	id, err := f.v.Verify(context.Background(), f.token(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if id.Email != "bob@example.com" || id.Subject != "accounts.google.com:1" || id.Domain != "example.com" {
		t.Errorf("identity = %+v", id)
	}
	// Audience may also arrive as an array.
	if _, err := f.v.Verify(context.Background(), f.token(t, func(c map[string]any) { c["aud"] = []string{"other", aud} })); err != nil {
		t.Errorf("array aud: %v", err)
	}
}

func TestIAPVerifyRejects(t *testing.T) {
	f := newIAPFixture(t)
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	tests := map[string]string{
		"wrong audience":  f.token(t, func(c map[string]any) { c["aud"] = "/projects/999/apps/x" }),
		"wrong issuer":    f.token(t, func(c map[string]any) { c["iss"] = "https://evil.example" }),
		"expired":         f.token(t, func(c map[string]any) { c["exp"] = f.now.Add(-time.Minute).Unix() }),
		"future iat":      f.token(t, func(c map[string]any) { c["iat"] = f.now.Add(time.Hour).Unix() }),
		"missing exp":     f.token(t, func(c map[string]any) { delete(c, "exp") }),
		"missing email":   f.token(t, func(c map[string]any) { delete(c, "email") }),
		"wrong key":       sign(t, other, map[string]any{"alg": "ES256", "kid": "k1"}, f.claims()),
		"unknown kid":     sign(t, f.key, map[string]any{"alg": "ES256", "kid": "nope"}, f.claims()),
		"alg none":        sign(t, f.key, map[string]any{"alg": "none", "kid": "k1"}, f.claims()),
		"alg HS256":       sign(t, f.key, map[string]any{"alg": "HS256", "kid": "k1"}, f.claims()),
		"malformed":       "a.b",
		"empty signature": strings.Join(strings.Split(f.token(t, nil), ".")[:2], ".") + ".",
	}
	for name, tok := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := f.v.Verify(context.Background(), tok); err == nil {
				t.Fatal("expected error")
			}
		})
	}

	// Tampering with the claims after signing invalidates the signature.
	parts := strings.Split(f.token(t, nil), ".")
	c := f.claims()
	c["email"] = "admin@example.com"
	b, _ := json.Marshal(c)
	parts[1] = base64.RawURLEncoding.EncodeToString(b)
	if _, err := f.v.Verify(context.Background(), strings.Join(parts, ".")); err == nil {
		t.Error("tampered claims accepted")
	}
}

func TestIAPKeyCaching(t *testing.T) {
	f := newIAPFixture(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := f.v.Verify(ctx, f.token(t, nil)); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.fetches.Load(); n != 1 {
		t.Errorf("fetches = %d, want 1 (keys should be cached)", n)
	}

	// Unknown kids can't force a refetch more than once per minute.
	bad := sign(t, f.key, map[string]any{"alg": "ES256", "kid": "nope"}, f.claims())
	for i := 0; i < 3; i++ {
		_, _ = f.v.Verify(ctx, bad)
	}
	if n := f.fetches.Load(); n != 1 {
		t.Errorf("fetches = %d after unknown kids, want 1", n)
	}

	// Key rotation: a new kid is picked up once the refresh window passes.
	f.now = f.now.Add(2 * time.Minute)
	f.kid = "k2"
	if _, err := f.v.Verify(ctx, f.token(t, nil)); err != nil {
		t.Fatalf("rotated key: %v", err)
	}
	if n := f.fetches.Load(); n != 2 {
		t.Errorf("fetches = %d after rotation, want 2", n)
	}
}

func TestIAPRequiresAudience(t *testing.T) {
	f := newIAPFixture(t)
	f.v.Audience = ""
	if _, err := f.v.Verify(context.Background(), f.token(t, nil)); err == nil {
		t.Fatal("verifier without audience must reject everything")
	}
}
