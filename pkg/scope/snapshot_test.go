package scope

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestScopeSnapshotsOwnTheirRuleSlices(t *testing.T) {
	rules := []Rule{{URL: regexp.MustCompile("example.com")}}
	s := &Scope{}
	s.SetRules(rules)
	rules[0] = Rule{}
	exported := s.Rules()
	exported[0] = Rule{}
	req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
	if !s.Match(req, nil) {
		t.Fatal("mutating input/output slices changed the stored scope")
	}
	snapshot := s.Snapshot()
	s.SetRules(nil)
	if !snapshot.Match(req, nil) || s.Match(req, nil) {
		t.Fatal("snapshot did not preserve its original scope")
	}
}
