package main

import (
	"crypto/subtle"
	"net/http"
	"slices"
	"strings"

	"github.com/gorilla/mux"
)

// authNoAuthPaths are the admin endpoints reachable without a Bearer token.
// They serve public values (the admin token itself and the public CA
// certificate — never the CA private key) and are reached by header-less
// browser requests (plain navigation, an <a> download), so requiring a token
// would make them unusable. Access is still gated by the default localhost-only
// bind.
var authNoAuthPaths = map[string]bool{
	"/api/token":  true,
	"/api/ca.crt": true,
}

// newAuthMiddleware requires a valid Bearer token on every /api/* route, with
// two exceptions: the header-less allowlist above, and the SSE stream
// (/api/events), which authenticates via a `token` query param because
// EventSource cannot set an Authorization header. Non-/api routes pass through
// untouched (the embedded UI is served from the same router).
func newAuthMiddleware(token string) mux.MiddlewareFunc {
	tokenBytes := []byte(token)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authNoAuthPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			if r.URL.Path == "/api/events" &&
				subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), tokenBytes) == 1 {
				next.ServeHTTP(w, r)
				return
			}

			if strings.HasPrefix(r.URL.Path, "/api/") {
				auth := r.Header.Get("Authorization")
				if !strings.HasPrefix(auth, "Bearer ") ||
					subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, "Bearer ")), tokenBytes) != 1 {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// newCSRFMiddleware rejects state-changing requests (POST/PUT/DELETE) that carry
// an Origin header not in allowedOrigins. Requests without an Origin header pass
// through, so non-browser clients (curl, the CLI) are unaffected; browsers
// always send Origin on cross-origin state-changers, which is what this blocks.
func newCSRFMiddleware(allowedOrigins []string) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodDelete:
				origin := r.Header.Get("Origin")
				if origin != "" && !slices.Contains(allowedOrigins, origin) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
