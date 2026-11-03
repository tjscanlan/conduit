package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// IAPAssertionHeader carries the signed JWT that Google Cloud IAP attaches
	// to every request it forwards. The unsigned x-goog-authenticated-user-*
	// headers must never be trusted on their own.
	IAPAssertionHeader = "X-Goog-Iap-Jwt-Assertion"

	// IAPPublicKeysURL serves IAP's ES256 signing keys as a JWK set.
	IAPPublicKeysURL = "https://www.gstatic.com/iap/verify/public_key-jwk"

	iapIssuer = "https://cloud.google.com/iap"

	iapClockSkew      = 30 * time.Second
	iapKeysMaxAge     = time.Hour
	iapKeysMinRefresh = time.Minute
)

// IAPIdentity is the verified identity from an IAP JWT.
type IAPIdentity struct {
	Email   string
	Subject string
	Domain  string // hosted domain (hd claim); empty for consumer accounts
}

// IAPVerifier validates IAP JWT assertions (ES256) against Google's
// published keys, enforcing issuer, audience, and expiry.
//
// The audience is "/projects/PROJECT_NUMBER/global/backendServices/SERVICE_ID"
// for load balancer backends or "/projects/PROJECT_NUMBER/apps/PROJECT_ID"
// for App Engine.
type IAPVerifier struct {
	Audience string
	KeysURL  string
	Client   *http.Client
	Now      func() time.Time

	mu        sync.Mutex
	keys      map[string]*ecdsa.PublicKey
	fetchedAt time.Time
}

// NewIAPVerifier returns a verifier for the given audience using Google's key set.
func NewIAPVerifier(audience string) *IAPVerifier {
	return &IAPVerifier{
		Audience: audience,
		KeysURL:  IAPPublicKeysURL,
		Client:   &http.Client{Timeout: 10 * time.Second},
		Now:      time.Now,
	}
}

type iapHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type iapClaims struct {
	Iss   string          `json:"iss"`
	Aud   json.RawMessage `json:"aud"` // string or array of strings
	Sub   string          `json:"sub"`
	Email string          `json:"email"`
	Hd    string          `json:"hd"`
	Exp   int64           `json:"exp"`
	Iat   int64           `json:"iat"`
}

// Verify checks a raw JWT from the IAP assertion header and returns the identity.
func (v *IAPVerifier) Verify(ctx context.Context, token string) (*IAPIdentity, error) {
	if v.Audience == "" {
		return nil, errors.New("iap: audience not configured")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("iap: malformed token")
	}

	var hdr iapHeader
	if err := decodeSegment(parts[0], &hdr); err != nil {
		return nil, fmt.Errorf("iap: header: %w", err)
	}
	if hdr.Alg != "ES256" {
		return nil, fmt.Errorf("iap: unexpected alg %q", hdr.Alg)
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return nil, errors.New("iap: malformed signature")
	}
	key, err := v.key(ctx, hdr.Kid)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(key, digest[:], r, s) {
		return nil, errors.New("iap: invalid signature")
	}

	var c iapClaims
	if err := decodeSegment(parts[1], &c); err != nil {
		return nil, fmt.Errorf("iap: claims: %w", err)
	}
	now := v.now()
	if c.Iss != iapIssuer {
		return nil, fmt.Errorf("iap: unexpected issuer %q", c.Iss)
	}
	if !audienceMatches(c.Aud, v.Audience) {
		return nil, errors.New("iap: audience mismatch")
	}
	if c.Exp == 0 || now.After(time.Unix(c.Exp, 0).Add(iapClockSkew)) {
		return nil, errors.New("iap: token expired")
	}
	if c.Iat == 0 || time.Unix(c.Iat, 0).After(now.Add(iapClockSkew)) {
		return nil, errors.New("iap: token issued in the future")
	}
	if c.Email == "" || c.Sub == "" {
		return nil, errors.New("iap: token missing email or sub")
	}
	return &IAPIdentity{Email: c.Email, Subject: c.Sub, Domain: c.Hd}, nil
}

func (v *IAPVerifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

// key returns the public key for kid, refreshing the cached key set when it
// is stale or kid is unknown (rate-limited so bad kids can't force refetches).
func (v *IAPVerifier) key(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := v.now()
	k, ok := v.keys[kid]
	stale := now.Sub(v.fetchedAt) > iapKeysMaxAge
	canRefresh := now.Sub(v.fetchedAt) > iapKeysMinRefresh
	if (ok && !stale) || (!ok && !canRefresh) {
		if !ok {
			return nil, fmt.Errorf("iap: unknown key id %q", kid)
		}
		return k, nil
	}

	keys, err := v.fetchKeys(ctx)
	if err != nil {
		if ok {
			return k, nil // keep serving the cached key if Google is unreachable
		}
		return nil, err
	}
	v.keys, v.fetchedAt = keys, now
	if k, ok = keys[kid]; !ok {
		return nil, fmt.Errorf("iap: unknown key id %q", kid)
	}
	return k, nil
}

func (v *IAPVerifier) fetchKeys(ctx context.Context) (map[string]*ecdsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.KeysURL, nil)
	if err != nil {
		return nil, err
	}
	client := v.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("iap: fetch keys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("iap: fetch keys: status %d", resp.StatusCode)
	}

	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			Kid string `json:"kid"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&set); err != nil {
		return nil, fmt.Errorf("iap: decode keys: %w", err)
	}

	keys := make(map[string]*ecdsa.PublicKey, len(set.Keys))
	for _, jwk := range set.Keys {
		if jwk.Kty != "EC" || jwk.Crv != "P-256" || jwk.Kid == "" {
			continue
		}
		x, errX := base64.RawURLEncoding.DecodeString(jwk.X)
		y, errY := base64.RawURLEncoding.DecodeString(jwk.Y)
		if errX != nil || errY != nil {
			continue
		}
		pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !pub.Curve.IsOnCurve(pub.X, pub.Y) {
			continue
		}
		keys[jwk.Kid] = pub
	}
	if len(keys) == 0 {
		return nil, errors.New("iap: key set contained no usable keys")
	}
	return keys, nil
}

func decodeSegment(seg string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func audienceMatches(raw json.RawMessage, want string) bool {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one == want
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		for _, a := range many {
			if a == want {
				return true
			}
		}
	}
	return false
}
