package reqlog

import (
	"net/http"
	"testing"
)

func TestRedactHeaders(t *testing.T) {
	t.Run("masks named headers case-insensitively", func(t *testing.T) {
		h := http.Header{}
		h.Set("Authorization", "Bearer secret-token")
		h.Set("Cookie", "session=abc123")
		h.Set("X-Custom", "keep-me")

		redactHeaders(h, []string{"authorization", "COOKIE"})

		if got := h.Get("Authorization"); got != redactMask {
			t.Errorf("Authorization = %q, want %q", got, redactMask)
		}
		if got := h.Get("Cookie"); got != redactMask {
			t.Errorf("Cookie = %q, want %q", got, redactMask)
		}
		if got := h.Get("X-Custom"); got != "keep-me" {
			t.Errorf("X-Custom = %q, want it left untouched", got)
		}
	})

	t.Run("collapses multi-value headers to a single mask", func(t *testing.T) {
		h := http.Header{}
		h.Add("Set-Cookie", "a=1")
		h.Add("Set-Cookie", "b=2")

		redactHeaders(h, []string{"Set-Cookie"})

		if vals := h.Values("Set-Cookie"); len(vals) != 1 || vals[0] != redactMask {
			t.Errorf("Set-Cookie = %v, want [%q]", vals, redactMask)
		}
	})

	t.Run("absent header is a no-op", func(t *testing.T) {
		h := http.Header{}
		h.Set("Content-Type", "application/json")

		redactHeaders(h, []string{"Authorization"})

		if _, ok := h["Authorization"]; ok {
			t.Error("Authorization should not have been added")
		}
		if got := h.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want it untouched", got)
		}
	})

	t.Run("empty name list leaves everything", func(t *testing.T) {
		h := http.Header{}
		h.Set("Authorization", "Bearer x")

		redactHeaders(h, nil)

		if got := h.Get("Authorization"); got != "Bearer x" {
			t.Errorf("Authorization = %q, want it untouched", got)
		}
	})
}
