package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/tjscanlan/conduit/internal/auth"
)

const (
	testUser     = "alice"
	testPassword = "correct horse battery staple"
	testAudience = "/projects/123/global/backendServices/456"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// newTestRouter builds the real router in the given mode. ClickHouse is not
// available, so tests exercise routes that don't query it (/logs, /metrics).
func newTestRouter(t *testing.T, mode string) (http.Handler, *uiAuth) {
	t.Helper()
	tmpls := parseTemplates()
	cfg := &Config{AuthMode: mode, SessionTTL: time.Hour, SessionKey: strings.Repeat("k", 32)}

	if mode == authModeLogin {
		hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		cfg.UsersFile = filepath.Join(t.TempDir(), "users")
		if err := os.WriteFile(cfg.UsersFile, []byte(testUser+":"+string(hash)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if mode == authModeIAP {
		cfg.IAPAudience = testAudience
	}

	a, err := newUIAuth(cfg, tmpls, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{ch: &fakeSpans{}, tmpls: tmpls, logger: testLogger()}
	return newRouter(a, h), a
}

func do(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func loginRequest(user, pass, next string) *http.Request {
	form := url.Values{"username": {user}, "password": {pass}, "next": {next}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	return req
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "__Host-conduit_session" {
			return c
		}
	}
	t.Fatalf("no session cookie set; headers: %v", rec.Header())
	return nil
}

func TestLoginModeRedirectsUnauthenticated(t *testing.T) {
	router, _ := newTestRouter(t, authModeLogin)

	rec := do(router, httptest.NewRequest(http.MethodGet, "/logs?x=1", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if got, want := rec.Header().Get("Location"), "/login?next="+url.QueryEscape("/logs?x=1"); got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	// htmx polling requests get a client-side redirect instead of a swapped-in login page.
	req := httptest.NewRequest(http.MethodGet, "/traces", nil)
	req.Header.Set("HX-Request", "true")
	rec = do(router, req)
	if rec.Code != http.StatusUnauthorized || !strings.HasPrefix(rec.Header().Get("HX-Redirect"), "/login?next=") {
		t.Errorf("htmx: status = %d, HX-Redirect = %q", rec.Code, rec.Header().Get("HX-Redirect"))
	}
}

func TestPublicRoutes(t *testing.T) {
	router, _ := newTestRouter(t, authModeLogin)
	for _, path := range []string{"/healthz", "/static/css/app.css", "/login"} {
		if rec := do(router, httptest.NewRequest(http.MethodGet, path, nil)); rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
		}
	}
}

func TestLoginFlow(t *testing.T) {
	router, _ := newTestRouter(t, authModeLogin)

	rec := do(router, loginRequest(testUser, "wrong", "/logs"))
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "Invalid username or password") {
		t.Fatalf("bad password: status = %d", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("bad password must not set a cookie")
	}
	if rec := do(router, loginRequest("mallory", testPassword, "/logs")); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown user: status = %d", rec.Code)
	}

	rec = do(router, loginRequest(testUser, testPassword, "/logs"))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/logs" {
		t.Fatalf("login: status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
	c := sessionCookie(t, rec)
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Errorf("cookie flags: %+v", c)
	}

	req := httptest.NewRequest(http.MethodGet, "/logs", nil)
	req.AddCookie(c)
	rec = do(router, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated GET /logs = %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, testUser) || !strings.Contains(body, `action="/logout"`) {
		t.Error("nav should show the signed-in user and a sign-out form")
	}

	// A tampered cookie is rejected.
	bad := *c
	bad.Value = strings.Replace(c.Value, ".", "x.", 1)
	req = httptest.NewRequest(http.MethodGet, "/logs", nil)
	req.AddCookie(&bad)
	if rec := do(router, req); rec.Code != http.StatusSeeOther {
		t.Errorf("tampered cookie: status = %d, want 303", rec.Code)
	}

	// Logout expires the cookie.
	req = httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(c)
	rec = do(router, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("logout: status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
	if out := sessionCookie(t, rec); out.MaxAge >= 0 || out.Value != "" {
		t.Errorf("logout cookie not cleared: %+v", out)
	}
}

func TestLoginRejectsCrossOrigin(t *testing.T) {
	router, _ := newTestRouter(t, authModeLogin)

	req := loginRequest(testUser, testPassword, "/")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if rec := do(router, req); rec.Code != http.StatusForbidden {
		t.Errorf("Sec-Fetch-Site cross-site: status = %d, want 403", rec.Code)
	}

	req = loginRequest(testUser, testPassword, "/")
	req.Header.Del("Sec-Fetch-Site")
	req.Header.Set("Origin", "https://evil.example")
	if rec := do(router, req); rec.Code != http.StatusForbidden {
		t.Errorf("foreign Origin: status = %d, want 403", rec.Code)
	}
}

func TestLoginRateLimited(t *testing.T) {
	router, _ := newTestRouter(t, authModeLogin)
	for i := 0; i < 5; i++ {
		do(router, loginRequest(testUser, "wrong", "/"))
	}
	rec := do(router, loginRequest(testUser, testPassword, "/"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("6th attempt: status = %d, want 429", rec.Code)
	}
}

func TestSafeNext(t *testing.T) {
	tests := map[string]string{
		"":                     defaultLandingPath,
		"/logs":                "/logs",
		"/traces/abc?x=1":      "/traces/abc?x=1",
		"//evil.example":       defaultLandingPath,
		`/\evil.example`:       defaultLandingPath,
		"https://evil.example": defaultLandingPath,
		"javascript:alert(1)":  defaultLandingPath,
		"/login":               defaultLandingPath,
	}
	for in, want := range tests {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNoneMode(t *testing.T) {
	router, _ := newTestRouter(t, authModeNone)
	if rec := do(router, httptest.NewRequest(http.MethodGet, "/logs", nil)); rec.Code != http.StatusOK {
		t.Fatalf("GET /logs = %d, want 200", rec.Code)
	}
}

// signES256 builds a JWT the way IAP does.
func signES256(t *testing.T, key *ecdsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	input := enc(map[string]string{"alg": "ES256", "kid": kid}) + "." + enc(claims)
	digest := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func jwksServer(t *testing.T, kid string, pub *ecdsa.PublicKey) *httptest.Server {
	t.Helper()
	b64 := func(i *big.Int) string { return base64.RawURLEncoding.EncodeToString(i.FillBytes(make([]byte, 32))) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"keys":[{"kty":"EC","crv":"P-256","alg":"ES256","kid":%q,"x":%q,"y":%q}]}`, kid, b64(pub.X), b64(pub.Y))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestIAPMode(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	jwks := jwksServer(t, "k1", &key.PublicKey)

	router, a := newTestRouter(t, authModeIAP)
	a.iap.KeysURL = jwks.URL
	a.iap.Client = jwks.Client()

	now := time.Now()
	token := signES256(t, key, "k1", map[string]any{
		"iss": "https://cloud.google.com/iap", "aud": testAudience,
		"sub": "accounts.google.com:1", "email": "bob@example.com",
		"iat": now.Unix(), "exp": now.Add(10 * time.Minute).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/logs", nil)
	req.Header.Set(auth.IAPAssertionHeader, token)
	rec := do(router, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid IAP assertion: status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "bob@example.com") {
		t.Error("nav should show the IAP email")
	}

	// The unsigned email header alone must not authenticate.
	req = httptest.NewRequest(http.MethodGet, "/logs", nil)
	req.Header.Set("X-Goog-Authenticated-User-Email", "accounts.google.com:bob@example.com")
	if rec := do(router, req); rec.Code != http.StatusUnauthorized {
		t.Errorf("no assertion: status = %d, want 401", rec.Code)
	}

	// Health checks bypass IAP on GCP and must stay reachable.
	if rec := do(router, httptest.NewRequest(http.MethodGet, "/healthz", nil)); rec.Code != http.StatusOK {
		t.Errorf("/healthz = %d, want 200", rec.Code)
	}

	// The login form isn't used under IAP.
	if rec := do(router, httptest.NewRequest(http.MethodGet, "/login", nil)); rec.Code != http.StatusSeeOther {
		t.Errorf("GET /login = %d, want redirect", rec.Code)
	}

	// Sign out goes through IAP's own logout endpoint.
	req = httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec = do(router, req)
	if rec.Header().Get("Location") != iapLogoutPath {
		t.Errorf("logout Location = %q, want %q", rec.Header().Get("Location"), iapLogoutPath)
	}
}

func TestHashPassword(t *testing.T) {
	var out, errOut strings.Builder
	if code := hashPassword(strings.NewReader(testPassword+"\n"), &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	s, err := auth.ParseUsers(strings.NewReader("alice:" + out.String()))
	if err != nil {
		t.Fatal(err)
	}
	if !s.Authenticate("alice", testPassword) {
		t.Error("hash does not verify")
	}
	if code := hashPassword(strings.NewReader("short\n"), &out, &errOut); code == 0 {
		t.Error("short password accepted")
	}
}
