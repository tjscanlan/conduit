package auth

import (
	"net/http"
	"strings"
)

// BearerTokenMiddleware rejects requests without a valid Authorization: Bearer <token> header.
// Attach after routing — skip for /static/ and /health if needed.
func BearerTokenMiddleware(store *TokenStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		if _, err := store.Validate(token); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
