package scope_test

import (
	"bytes"
	"encoding/gob"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"testing"

	"github.com/lorenzocamilli/lynx/pkg/scope"
)

func mustRequest(t *testing.T, rawURL string, headers map[string]string) *http.Request {
	t.Helper()

	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}

	req := &http.Request{URL: u, Header: http.Header{}}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return req
}

func TestScopeRulesRoundTrip(t *testing.T) {
	t.Parallel()

	s := &scope.Scope{}

	if got := s.Rules(); len(got) != 0 {
		t.Fatalf("expected no rules initially, got %d", len(got))
	}

	rules := []scope.Rule{
		{URL: regexp.MustCompile(`example\.com`)},
		{Body: regexp.MustCompile(`secret`)},
	}
	s.SetRules(rules)

	got := s.Rules()
	if len(got) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(got))
	}
}

func TestScopeRulesConcurrentAccess(t *testing.T) {
	t.Parallel()

	s := &scope.Scope{}

	var wg sync.WaitGroup

	for range 50 {
		wg.Add(2)

		go func() {
			defer wg.Done()

			s.SetRules([]scope.Rule{{URL: regexp.MustCompile("a")}})
		}()

		go func() {
			defer wg.Done()

			_ = s.Rules()
		}()
	}

	wg.Wait()
}

func TestScopeMatch(t *testing.T) {
	t.Parallel()

	t.Run("no rules never matches", func(t *testing.T) {
		t.Parallel()

		s := &scope.Scope{}
		req := mustRequest(t, "https://example.com/foo", nil)

		if s.Match(req, nil) {
			t.Fatal("expected no match with zero rules")
		}
	})

	t.Run("matches when any rule matches", func(t *testing.T) {
		t.Parallel()

		s := &scope.Scope{}
		s.SetRules([]scope.Rule{
			{URL: regexp.MustCompile(`nomatch\.test`)},
			{URL: regexp.MustCompile(`example\.com`)},
		})
		req := mustRequest(t, "https://example.com/foo", nil)

		if !s.Match(req, nil) {
			t.Fatal("expected match, got none")
		}
	})

	t.Run("no rule matches", func(t *testing.T) {
		t.Parallel()

		s := &scope.Scope{}
		s.SetRules([]scope.Rule{
			{URL: regexp.MustCompile(`nomatch\.test`)},
		})
		req := mustRequest(t, "https://example.com/foo", nil)

		if s.Match(req, nil) {
			t.Fatal("expected no match")
		}
	})
}

func TestRuleMatchURL(t *testing.T) {
	t.Parallel()

	rule := scope.Rule{URL: regexp.MustCompile(`^https://example\.com/admin`)}

	if !rule.Match(mustRequest(t, "https://example.com/admin/panel", nil), nil) {
		t.Fatal("expected URL match")
	}

	if rule.Match(mustRequest(t, "https://example.com/public", nil), nil) {
		t.Fatal("expected no URL match")
	}
}

func TestRuleMatchHeader(t *testing.T) {
	t.Parallel()

	t.Run("key only", func(t *testing.T) {
		t.Parallel()

		rule := scope.Rule{Header: scope.Header{Key: regexp.MustCompile(`(?i)^x-api-key$`)}}

		if !rule.Match(mustRequest(t, "https://example.com", map[string]string{"X-Api-Key": "anything"}), nil) {
			t.Fatal("expected header key match")
		}

		if rule.Match(mustRequest(t, "https://example.com", map[string]string{"X-Other": "anything"}), nil) {
			t.Fatal("expected no header key match")
		}
	})

	t.Run("value only", func(t *testing.T) {
		t.Parallel()

		rule := scope.Rule{Header: scope.Header{Value: regexp.MustCompile(`secret-token`)}}

		if !rule.Match(mustRequest(t, "https://example.com", map[string]string{"Authorization": "secret-token"}), nil) {
			t.Fatal("expected header value match")
		}

		if rule.Match(mustRequest(t, "https://example.com", map[string]string{"Authorization": "public"}), nil) {
			t.Fatal("expected no header value match")
		}
	})

	t.Run("key and value both required", func(t *testing.T) {
		t.Parallel()

		rule := scope.Rule{Header: scope.Header{
			Key:   regexp.MustCompile(`(?i)^cookie$`),
			Value: regexp.MustCompile(`session`),
		}}

		// Key matches on one header, value matches on a different header — neither
		// header satisfies both, so this must not match.
		req := mustRequest(t, "https://example.com", map[string]string{
			"Cookie":        "id=1",
			"X-Has-Session": "session=abc",
		})
		if rule.Match(req, nil) {
			t.Fatal("expected no match when key and value match different headers")
		}

		// Same header satisfies both key and value.
		req = mustRequest(t, "https://example.com", map[string]string{
			"Cookie": "session=abc",
		})
		if !rule.Match(req, nil) {
			t.Fatal("expected match when key and value match the same header")
		}
	})
}

func TestRuleMatchBody(t *testing.T) {
	t.Parallel()

	rule := scope.Rule{Body: regexp.MustCompile(`password`)}

	if !rule.Match(mustRequest(t, "https://example.com", nil), []byte(`{"password":"hunter2"}`)) {
		t.Fatal("expected body match")
	}

	if rule.Match(mustRequest(t, "https://example.com", nil), []byte(`{"user":"bob"}`)) {
		t.Fatal("expected no body match")
	}
}

func TestRuleMatchEmptyRule(t *testing.T) {
	t.Parallel()

	rule := scope.Rule{}

	if rule.Match(mustRequest(t, "https://example.com/anything", nil), []byte("anything")) {
		t.Fatal("expected an empty rule to never match")
	}
}

func TestRuleMarshalUnmarshalBinaryRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("all fields set", func(t *testing.T) {
		t.Parallel()

		rule := scope.Rule{
			URL: regexp.MustCompile(`example\.com`),
			Header: scope.Header{
				Key:   regexp.MustCompile(`(?i)^cookie$`),
				Value: regexp.MustCompile(`session`),
			},
			Body: regexp.MustCompile(`password`),
		}

		data, err := rule.MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary failed: %v", err)
		}

		var got scope.Rule
		if err := got.UnmarshalBinary(data); err != nil {
			t.Fatalf("UnmarshalBinary failed: %v", err)
		}

		if got.URL.String() != rule.URL.String() {
			t.Errorf("URL = %q, want %q", got.URL.String(), rule.URL.String())
		}

		if got.Header.Key.String() != rule.Header.Key.String() {
			t.Errorf("Header.Key = %q, want %q", got.Header.Key.String(), rule.Header.Key.String())
		}

		if got.Header.Value.String() != rule.Header.Value.String() {
			t.Errorf("Header.Value = %q, want %q", got.Header.Value.String(), rule.Header.Value.String())
		}

		if got.Body.String() != rule.Body.String() {
			t.Errorf("Body = %q, want %q", got.Body.String(), rule.Body.String())
		}
	})

	t.Run("no fields set", func(t *testing.T) {
		t.Parallel()

		rule := scope.Rule{}

		data, err := rule.MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary failed: %v", err)
		}

		got := scope.Rule{URL: regexp.MustCompile(`stale`)}
		if err := got.UnmarshalBinary(data); err != nil {
			t.Fatalf("UnmarshalBinary failed: %v", err)
		}

		if got.URL != nil || got.Header.Key != nil || got.Header.Value != nil || got.Body != nil {
			t.Errorf("expected all fields nil after round-tripping an empty rule, got %+v", got)
		}
	})
}

func TestRuleUnmarshalBinaryErrors(t *testing.T) {
	t.Parallel()

	t.Run("malformed gob data", func(t *testing.T) {
		t.Parallel()

		var rule scope.Rule
		if err := rule.UnmarshalBinary([]byte("not valid gob data")); err == nil {
			t.Fatal("expected an error decoding malformed gob data")
		}
	})

	t.Run("invalid regexp pattern", func(t *testing.T) {
		t.Parallel()

		// Mirrors the unexported ruleDTO shape in scope.go so we can encode a
		// payload with a URL pattern that fails to compile as a regexp.
		type headerDTO struct {
			Key   string
			Value string
		}

		type ruleDTO struct {
			URL    string
			Header headerDTO
			Body   string
		}

		buf := &bytes.Buffer{}
		if err := gob.NewEncoder(buf).Encode(ruleDTO{URL: `(unterminated`}); err != nil {
			t.Fatalf("failed to encode test payload: %v", err)
		}

		var rule scope.Rule
		if err := rule.UnmarshalBinary(buf.Bytes()); err == nil {
			t.Fatal("expected an error decoding an invalid regexp pattern")
		}
	})
}
