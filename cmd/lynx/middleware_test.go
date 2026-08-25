package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const testToken = "s3cr3t-admin-token"

// okHandler records that it was reached and writes 200. The auth/CSRF
// middleware must only call next when the request is authorized, so reaching
// this handler is the signal that a request was allowed through.
func okHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestAuthMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		authHeader string
		wantStatus int
		wantNext   bool
	}{
		{
			name:       "api route without token is unauthorized",
			method:     http.MethodPost,
			path:       "/api/graphql/",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
			wantNext:   false,
		},
		{
			name:       "api route with valid token passes",
			method:     http.MethodPost,
			path:       "/api/graphql/",
			authHeader: "Bearer " + testToken,
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "api route with wrong token is unauthorized",
			method:     http.MethodPost,
			path:       "/api/graphql/",
			authHeader: "Bearer wrong-token",
			wantStatus: http.StatusUnauthorized,
			wantNext:   false,
		},
		{
			name:       "api route with malformed auth header is unauthorized",
			method:     http.MethodPost,
			path:       "/api/graphql/",
			authHeader: testToken, // missing "Bearer " prefix
			wantStatus: http.StatusUnauthorized,
			wantNext:   false,
		},
		{
			name:       "settings route without token is unauthorized",
			method:     http.MethodGet,
			path:       "/api/settings",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
			wantNext:   false,
		},
		{
			name:       "token endpoint is reachable without auth",
			method:     http.MethodGet,
			path:       "/api/token",
			authHeader: "",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "ca.crt endpoint is reachable without auth",
			method:     http.MethodGet,
			path:       "/api/ca.crt",
			authHeader: "",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "non-api route passes through untouched",
			method:     http.MethodGet,
			path:       "/proxy/logs",
			authHeader: "",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
	}

	mw := newAuthMiddleware(testToken)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			handler := mw(okHandler(&reached))

			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if reached != tt.wantNext {
				t.Errorf("next reached = %v, want %v", reached, tt.wantNext)
			}
		})
	}
}

func TestAuthMiddlewareEventsQueryToken(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantNext   bool
	}{
		{
			name:       "valid query token passes",
			query:      "?token=" + testToken,
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "wrong query token is unauthorized",
			query:      "?token=nope",
			wantStatus: http.StatusUnauthorized,
			wantNext:   false,
		},
		{
			name:       "missing query token is unauthorized",
			query:      "",
			wantStatus: http.StatusUnauthorized,
			wantNext:   false,
		},
	}

	mw := newAuthMiddleware(testToken)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			handler := mw(okHandler(&reached))

			// EventSource cannot set headers, so no Authorization header is sent.
			req := httptest.NewRequest(http.MethodGet, "/api/events"+tt.query, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if reached != tt.wantNext {
				t.Errorf("next reached = %v, want %v", reached, tt.wantNext)
			}
		})
	}
}

// TestAuthMiddlewareEventsRejectsBogusTokenViaHeader guards against a regression
// where the /api/events allowance leaks to the Bearer path: an empty/wrong query
// token with no valid header must still be rejected.
func TestAuthMiddlewareEventsRejectsHeaderBypass(t *testing.T) {
	mw := newAuthMiddleware(testToken)
	reached := false
	handler := mw(okHandler(&reached))

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if reached {
		t.Error("next was reached; events must reject an invalid query token")
	}
}

func TestCSRFMiddleware(t *testing.T) {
	allowed := []string{
		"http://127.0.0.1:8080",
		"http://localhost:8080",
	}

	tests := []struct {
		name       string
		method     string
		origin     string
		wantStatus int
		wantNext   bool
	}{
		{
			name:       "state-changing request with no origin passes (non-browser client)",
			method:     http.MethodPost,
			origin:     "",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "state-changing request with allowed origin passes",
			method:     http.MethodPost,
			origin:     "http://127.0.0.1:8080",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "state-changing request with cross-origin is forbidden",
			method:     http.MethodPost,
			origin:     "http://evil.example.com",
			wantStatus: http.StatusForbidden,
			wantNext:   false,
		},
		{
			name:       "PUT with cross-origin is forbidden",
			method:     http.MethodPut,
			origin:     "http://evil.example.com",
			wantStatus: http.StatusForbidden,
			wantNext:   false,
		},
		{
			name:       "DELETE with cross-origin is forbidden",
			method:     http.MethodDelete,
			origin:     "http://evil.example.com",
			wantStatus: http.StatusForbidden,
			wantNext:   false,
		},
		{
			name:       "GET with cross-origin passes (not state-changing)",
			method:     http.MethodGet,
			origin:     "http://evil.example.com",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
	}

	mw := newCSRFMiddleware(allowed)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			handler := mw(okHandler(&reached))

			req := httptest.NewRequest(tt.method, "/api/settings", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if reached != tt.wantNext {
				t.Errorf("next reached = %v, want %v", reached, tt.wantNext)
			}
		})
	}
}
