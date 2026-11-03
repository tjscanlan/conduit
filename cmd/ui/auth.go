package main

import (
	"context"
	"crypto/rand"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/your-org/agent-observability/internal/auth"
)

const (
	// IAP's logout endpoint: clears the IAP session cookie for this app.
	iapLogoutPath = "/_gcp_iap/clear_login_cookie"

	defaultLandingPath = "/traces"
	maxLoginFormBytes  = 8 << 10
)

// uiUser is the authenticated viewer of the UI.
type uiUser struct {
	Name   string // username, or email under IAP
	Source string // authModeLogin, authModeIAP, or authModeNone
}

type uiUserKey struct{}

func withUIUser(ctx context.Context, u *uiUser) context.Context {
	return context.WithValue(ctx, uiUserKey{}, u)
}

func uiUserFromContext(ctx context.Context) *uiUser {
	u, _ := ctx.Value(uiUserKey{}).(*uiUser)
	return u
}

// uiAuth authenticates UI requests according to the configured mode.
type uiAuth struct {
	mode   string
	tmpls  *template.Template
	logger *slog.Logger

	// login mode
	users      *auth.UserStore
	sessions   *auth.SessionCodec
	cookieName string
	secure     bool
	limiter    *loginLimiter

	// iap mode
	iap *auth.IAPVerifier
}

func newUIAuth(cfg *Config, tmpls *template.Template, logger *slog.Logger) (*uiAuth, error) {
	a := &uiAuth{mode: cfg.AuthMode, tmpls: tmpls, logger: logger}
	switch cfg.AuthMode {
	case authModeLogin:
		users, err := auth.LoadUsersFile(cfg.UsersFile)
		if err != nil {
			return nil, err
		}
		key := []byte(cfg.SessionKey)
		if len(key) == 0 {
			key = make([]byte, 32)
			if _, err := rand.Read(key); err != nil {
				return nil, err
			}
			logger.Warn("UI_SESSION_KEY not set: using a random key; sessions will not survive restarts or work across replicas")
		}
		sessions, err := auth.NewSessionCodec(key, cfg.SessionTTL)
		if err != nil {
			return nil, err
		}
		a.users, a.sessions = users, sessions
		a.secure = !cfg.CookieInsecure
		a.cookieName = "conduit_session"
		if a.secure {
			a.cookieName = "__Host-conduit_session" // browser-enforced: Secure, Path=/, no Domain
		}
		a.limiter = newLoginLimiter(5, 12*time.Second)
	case authModeIAP:
		a.iap = auth.NewIAPVerifier(cfg.IAPAudience)
	case authModeNone:
		logger.Warn("UI_AUTH_MODE=none: the UI is unauthenticated; use only for local development")
	}
	return a, nil
}

// register adds the public auth routes and wraps protected with authentication.
func (a *uiAuth) register(mux *http.ServeMux, protected http.Handler) {
	mux.HandleFunc("GET /login", a.loginPage)
	mux.HandleFunc("POST /login", a.loginSubmit)
	mux.HandleFunc("POST /logout", a.logout)
	mux.Handle("/", a.require(protected))
}

// require rejects unauthenticated requests and attaches the user to the context.
func (a *uiAuth) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := a.authenticate(r)
		if u == nil {
			a.deny(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(withUIUser(r.Context(), u)))
	})
}

func (a *uiAuth) authenticate(r *http.Request) *uiUser {
	switch a.mode {
	case authModeNone:
		return &uiUser{Name: "anonymous", Source: authModeNone}
	case authModeIAP:
		token := r.Header.Get(auth.IAPAssertionHeader)
		if token == "" {
			a.logger.Warn("request without IAP assertion: is the UI reachable without going through IAP?", "remote", r.RemoteAddr)
			return nil
		}
		id, err := a.iap.Verify(r.Context(), token)
		if err != nil {
			a.logger.Warn("iap assertion rejected", "err", err, "remote", r.RemoteAddr)
			return nil
		}
		return &uiUser{Name: id.Email, Source: authModeIAP}
	case authModeLogin:
		c, err := r.Cookie(a.cookieName)
		if err != nil {
			return nil
		}
		s, err := a.sessions.Verify(c.Value)
		if err != nil {
			return nil
		}
		return &uiUser{Name: s.User, Source: authModeLogin}
	}
	return nil
}

func (a *uiAuth) deny(w http.ResponseWriter, r *http.Request) {
	if a.mode != authModeLogin {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	loginURL := "/login?next=" + url.QueryEscape(r.URL.RequestURI())
	switch {
	case r.Header.Get("HX-Request") == "true":
		// htmx would otherwise swap the login page into a fragment.
		w.Header().Set("HX-Redirect", loginURL)
		w.WriteHeader(http.StatusUnauthorized)
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		http.Redirect(w, r, loginURL, http.StatusSeeOther)
	default:
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
}

func (a *uiAuth) loginPage(w http.ResponseWriter, r *http.Request) {
	if a.mode != authModeLogin {
		http.Redirect(w, r, defaultLandingPath, http.StatusSeeOther)
		return
	}
	next := safeNext(r.URL.Query().Get("next"))
	if a.authenticate(r) != nil {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	a.renderLogin(w, http.StatusOK, next, "")
}

func (a *uiAuth) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if a.mode != authModeLogin {
		http.NotFound(w, r)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request rejected", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxLoginFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user := r.PostForm.Get("username")
	pass := r.PostForm.Get("password")
	next := safeNext(r.PostForm.Get("next"))

	if !a.limiter.allow("user:"+user) || !a.limiter.allow("ip:"+clientIP(r)) {
		a.logger.Warn("login rate limited", "user", user, "remote", r.RemoteAddr)
		a.renderLogin(w, http.StatusTooManyRequests, next, "Too many attempts. Wait a minute and try again.")
		return
	}
	if user == "" || !a.users.Authenticate(user, pass) {
		a.logger.Info("login failed", "user", user, "remote", r.RemoteAddr)
		a.renderLogin(w, http.StatusUnauthorized, next, "Invalid username or password.")
		return
	}

	value, err := a.sessions.Issue(user)
	if err != nil {
		a.logger.Error("issue session", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     a.cookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(a.sessions.TTL().Seconds()),
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
	})
	a.logger.Info("login succeeded", "user", user)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *uiAuth) logout(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request rejected", http.StatusForbidden)
		return
	}
	switch a.mode {
	case authModeIAP:
		http.Redirect(w, r, iapLogoutPath, http.StatusSeeOther)
	case authModeLogin:
		http.SetCookie(w, &http.Cookie{
			Name:     a.cookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   a.secure,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	default:
		http.Redirect(w, r, defaultLandingPath, http.StatusSeeOther)
	}
}

func (a *uiAuth) renderLogin(w http.ResponseWriter, status int, next, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := a.tmpls.ExecuteTemplate(w, "login", map[string]any{"Next": next, "Error": errMsg}); err != nil {
		a.logger.Error("render login", "err", err)
	}
}

// sameOrigin rejects cross-site form posts (login CSRF, forced logout).
// Browsers send Sec-Fetch-Site; older ones send Origin. Requests with
// neither are non-browser clients, which can't ride a victim's cookies.
func sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin" || site == "none"
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

// safeNext returns next if it is a local path, preventing open redirects.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.Contains(next, `\`) {
		return defaultLandingPath
	}
	u, err := url.Parse(next)
	if err != nil || u.IsAbs() || u.Host != "" || u.Path == "/login" {
		return defaultLandingPath
	}
	return next
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// loginLimiter is a token bucket per key (username or client IP).
type loginLimiter struct {
	mu       sync.Mutex
	burst    float64
	interval time.Duration // time to regain one attempt
	buckets  map[string]*loginBucket
	now      func() time.Time
}

type loginBucket struct {
	tokens float64
	last   time.Time
}

// maxLimiterKeys bounds memory under username-spraying; when exceeded the
// table is reset, which only ever errs toward allowing attempts.
const maxLimiterKeys = 10000

func newLoginLimiter(burst int, interval time.Duration) *loginLimiter {
	return &loginLimiter{
		burst:    float64(burst),
		interval: interval,
		buckets:  make(map[string]*loginBucket),
		now:      time.Now,
	}
}

func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= maxLimiterKeys {
			l.buckets = make(map[string]*loginBucket)
		}
		b = &loginBucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens += float64(now.Sub(b.last)) / float64(l.interval)
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
