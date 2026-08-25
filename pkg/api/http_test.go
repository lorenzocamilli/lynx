package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHTTPHandlerPlaygroundGated verifies the GraphQL Playground is only served
// when LYNX_DEV=1. In production it must 404 so the schema isn't exposed.
func TestHTTPHandlerPlaygroundGated(t *testing.T) {
	const endpoint = "/api/graphql/"

	tests := []struct {
		name       string
		devEnv     string
		wantStatus int
	}{
		{name: "playground disabled by default", devEnv: "", wantStatus: http.StatusNotFound},
		{name: "playground disabled when not 1", devEnv: "0", wantStatus: http.StatusNotFound},
		{name: "playground enabled when LYNX_DEV=1", devEnv: "1", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LYNX_DEV", tt.devEnv)

			handler := HTTPHandler(&Resolver{}, endpoint)

			req := httptest.NewRequest(http.MethodGet, endpoint, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("GET %s status = %d, want %d", endpoint, rec.Code, tt.wantStatus)
			}
		})
	}
}
