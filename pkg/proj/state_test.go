package proj

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid"

	"github.com/lorenzocamilli/lynx/pkg/filter"
	"github.com/lorenzocamilli/lynx/pkg/proxy/intercept"
	"github.com/lorenzocamilli/lynx/pkg/reqlog"
	"github.com/lorenzocamilli/lynx/pkg/scope"
	"github.com/lorenzocamilli/lynx/pkg/sender"
)

func TestConcurrentSettingsUpdatesPreserveAllFields(t *testing.T) {
	ts := newTestService(t)
	ctx := context.Background()
	id := ulid.ULID{1}
	for attempt := 0; attempt < 100; attempt++ {
		if err := ts.repo.UpsertProject(ctx, Project{ID: id}); err != nil {
			t.Fatal(err)
		}
		if _, err := ts.svc.OpenProject(ctx, id); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		for _, update := range []func() error{
			func() error { return ts.svc.SetRequestLogFindFilter(ctx, reqlog.FindRequestsFilter{OnlyInScope: true}) },
			func() error {
				return ts.svc.SetSenderRequestFindFilter(ctx, sender.FindRequestsFilter{OnlyInScope: true})
			},
			func() error { return ts.svc.UpdateInterceptSettings(ctx, intercept.Settings{RequestsEnabled: true}) },
		} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if err := update(); err != nil {
					t.Error(err)
				}
			}()
		}
		close(start)
		wg.Wait()
		stored, err := ts.repo.FindProjectByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if !stored.Settings.ReqLogOnlyFindInScope || !stored.Settings.SenderOnlyFindInScope || !stored.Settings.InterceptRequests {
			t.Fatalf("concurrent updates lost settings: %+v", stored.Settings)
		}
	}
}

func TestConcurrentProjectSwitchAndSettingsAccess(t *testing.T) {
	ts := newTestService(t)
	ctx := context.Background()
	ids := []ulid.ULID{{1}, {2}}
	for _, id := range ids {
		if err := ts.repo.UpsertProject(ctx, Project{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	checkError := func(err error) {
		if err != nil && !errors.Is(err, ErrNoProject) {
			t.Error(err)
		}
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				id := ids[i%len(ids)]
				if worker == 0 {
					if _, err := ts.svc.OpenProject(ctx, id); err != nil {
						t.Error(err)
					}
					if err := ts.svc.CloseProject(); err != nil {
						t.Error(err)
					}
				} else {
					_, err := ts.svc.ActiveProject(ctx)
					if err != nil && !errors.Is(err, ErrNoProject) {
						t.Error(err)
					}
					_ = ts.svc.IsProjectActive(id)
					checkError(ts.svc.SetSenderRequestFindFilter(ctx, sender.FindRequestsFilter{OnlyInScope: true}))
					checkError(ts.svc.SetRequestLogFindFilter(ctx, reqlog.FindRequestsFilter{OnlyInScope: true}))
					checkError(ts.svc.SetScopeRules(ctx, nil))
					checkError(ts.svc.UpdateInterceptSettings(ctx, intercept.Settings{}))
					_ = ts.senderSvc.FindReqsFilter()
					_ = ts.reqLogSvc.FindReqsFilter()
					_ = ts.reqLogSvc.ActiveProjectID()
				}
			}
		}(worker)
	}
	wg.Wait()
	if _, err := ts.svc.OpenProject(ctx, ids[1]); err != nil {
		t.Fatal(err)
	}
	if ts.senderSvc.FindReqsFilter().ProjectID != ids[1] || ts.reqLogSvc.FindReqsFilter().ProjectID != ids[1] {
		t.Fatal("services did not converge on the active project")
	}
}

type snapshotSenderRepository struct {
	sender.Repository
	find func(sender.FindRequestsFilter, *scope.Scope) ([]sender.Request, error)
}

func (r snapshotSenderRepository) FindSenderRequests(_ context.Context, filter sender.FindRequestsFilter, rules *scope.Scope) ([]sender.Request, error) {
	return r.find(filter, rules)
}

func TestSenderQueryRetainsFilterAndScopeAcrossProjectSwitch(t *testing.T) {
	ts := newTestService(t)
	ctx := context.Background()
	a, b := ulid.ULID{1}, ulid.ULID{2}
	for _, project := range []Project{
		{ID: a, Settings: Settings{SenderOnlyFindInScope: true, SenderSearchExpr: filter.StringLiteral{Value: "project A"}, ScopeRules: []scope.Rule{{URL: regexp.MustCompile("example.com")}}}},
		{ID: b, Settings: Settings{ScopeRules: []scope.Rule{{URL: regexp.MustCompile("other.test")}}}},
	} {
		if err := ts.repo.UpsertProject(ctx, project); err != nil {
			t.Fatal(err)
		}
	}
	// Reenter project switching from repository I/O. This both checks the
	// snapshot and proves the operation released its coordinator read lock.
	senderSvc := sender.NewService(sender.Config{Scope: ts.scopeSvc, Repository: snapshotSenderRepository{
		find: func(f sender.FindRequestsFilter, rules *scope.Scope) ([]sender.Request, error) {
			if _, err := ts.svc.OpenProject(ctx, b); err != nil {
				return nil, err
			}
			req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
			if f.ProjectID != a || !f.OnlyInScope || f.SearchExpr.(filter.StringLiteral).Value != "project A" || !rules.Match(req, nil) {
				t.Errorf("query mixed project settings: filter=%+v, scope=%+v", f, rules.Rules())
			}
			return nil, nil
		},
	}})
	var err error
	ts.svc, err = NewService(Config{Repository: ts.repo, SenderService: senderSvc, ReqLogService: ts.reqLogSvc, InterceptService: ts.svc.interceptSvc, Scope: ts.scopeSvc})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ts.svc.OpenProject(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := senderSvc.FindRequests(ctx, 10, 0); err != nil {
		t.Fatal(err)
	}
}

func TestSenderSnapshotWaitsForWholeProjectTransition(t *testing.T) {
	ts := newTestService(t)
	ctx := context.Background()
	observed := make(chan ulid.ULID, 1)
	senderSvc := sender.NewService(sender.Config{Scope: ts.scopeSvc, Repository: snapshotSenderRepository{
		find: func(f sender.FindRequestsFilter, rules *scope.Scope) ([]sender.Request, error) {
			if !rules.Match(httptest.NewRequest(http.MethodGet, "https://new.test", nil), nil) {
				t.Error("query observed a new project with old scope rules")
			}
			observed <- f.ProjectID
			return nil, nil
		},
	}})
	var err error
	ts.svc, err = NewService(Config{Repository: ts.repo, SenderService: senderSvc, ReqLogService: ts.reqLogSvc, InterceptService: ts.svc.interceptSvc, Scope: ts.scopeSvc})
	if err != nil {
		t.Fatal(err)
	}
	id := ulid.ULID{2}
	// Deliberately pause halfway through applying the same fields as OpenProject.
	ts.svc.mu.Lock()
	senderSvc.SetActiveProjectID(id)
	senderSvc.SetFindReqsFilter(sender.FindRequestsFilter{ProjectID: id})
	started, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		close(started)
		if _, err := senderSvc.FindRequests(ctx, 10, 0); err != nil {
			t.Error(err)
		}
	}()
	<-started
	var premature bool
	select {
	case <-observed:
		premature = true
	case <-time.After(50 * time.Millisecond):
	}
	ts.scopeSvc.SetRules([]scope.Rule{{URL: regexp.MustCompile("new.test")}})
	ts.svc.mu.Unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("query remained blocked after the transition")
	}
	if premature {
		t.Fatal("query entered the repository before project settings finished applying")
	}
	if got := <-observed; got != id {
		t.Fatalf("query project = %v, want %v", got, id)
	}
}
