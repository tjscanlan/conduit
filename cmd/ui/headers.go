package main

import "net/http"

// contentSecurityPolicy allows only same-origin scripts, styles, and XHR (htmx),
// and forbids inline script, framing, and <base> rewriting. Templates must not
// use inline event handlers or <style>/<script> blocks.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; " +
	"img-src 'self' data:; connect-src 'self'; object-src 'none'; " +
	"frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

// securityHeaders sets browser hardening headers on every response,
// including auth failures and static assets.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
