package proj

import (
	"context"
	"crypto/rand"
	"errors"
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

// fakeRepo is an in-memory Repository implementation for testing the
// orchestration logic in Service without a real database.
type fakeRepo struct {
	mu       sync.Mutex
	projects map[ulid.ULID]Project

	findErr   error
	upsertErr error
	deleteErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{projects: make(map[ulid.ULID]Project)}
}

func (r *fakeRepo) FindProjectByID(_ context.Context, id ulid.ULID) (Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.findErr != nil {
		return Project{}, r.findErr
	}

	p, ok := r.projects[id]
	if !ok {
		return Project{}, ErrProjectNotFound
	}

	return p, nil
}

func (r *fakeRepo) UpsertProject(_ context.Context, project Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.upsertErr != nil {
		return r.upsertErr
	}

	r.projects[project.ID] = project

	return nil
}

func (r *fakeRepo) DeleteProject(_ context.Context, id ulid.ULID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.deleteErr != nil {
		return r.deleteErr
	}

	delete(r.projects, id)

	return nil
}

func (r *fakeRepo) Projects(_ context.Context) ([]Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	projects := make([]Project, 0, len(r.projects))
	for _, p := range r.projects {
		projects = append(projects, p)
	}

	return projects, nil
}

func (r *fakeRepo) Close() error { return nil }

func newULID(t *testing.T) ulid.ULID {
	t.Helper()

	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
}

// testService bundles a Service with its fake repo and real (but
// storage-independent) collaborators, so state propagated into them can be
// observed via their exported getters.
type testService struct {
	svc       *Service
	repo      *fakeRepo
	reqLogSvc *reqlog.Service
	senderSvc *sender.Service
	scopeSvc  *scope.Scope
}

func newTestService(t *testing.T) testService {
	t.Helper()

	repo := newFakeRepo()
	scopeSvc := &scope.Scope{}
	reqLogSvc := reqlog.NewService(reqlog.Config{Scope: scopeSvc})
	senderSvc := sender.NewService(sender.Config{Scope: scopeSvc, ReqLogService: reqLogSvc})
	interceptSvc := intercept.NewService(intercept.Config{})

	svc, err := NewService(Config{
		Repository:       repo,
		InterceptService: interceptSvc,
		ReqLogService:    reqLogSvc,
		SenderService:    senderSvc,
		Scope:            scopeSvc,
	})
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	return testService{svc: svc, repo: repo, reqLogSvc: reqLogSvc, senderSvc: senderSvc, scopeSvc: scopeSvc}
}

func TestCreateProject(t *testing.T) {
	t.Parallel()

	t.Run("valid name", func(t *testing.T) {
		t.Parallel()

		ts := newTestService(t)

		project, err := ts.svc.CreateProject(context.Background(), "my project 1")
		if err != nil {
			t.Fatalf("CreateProject failed: %v", err)
		}

		if project.Name != "my project 1" {
			t.Errorf("Name = %q, want %q", project.Name, "my project 1")
		}

		if project.ID.Compare(ulid.ULID{}) == 0 {
			t.Error("expected a non-zero project ID")
		}

		stored, err := ts.repo.FindProjectByID(context.Background(), project.ID)
		if err != nil {
			t.Fatalf("expected project to be persisted: %v", err)
		}

		if stored.Name != project.Name {
			t.Errorf("persisted Name = %q, want %q", stored.Name, project.Name)
		}
	})

	t.Run("invalid name", func(t *testing.T) {
		t.Parallel()

		ts := newTestService(t)

		_, err := ts.svc.CreateProject(context.Background(), "invalid!@#")
		if !errors.Is(err, ErrInvalidName) {
			t.Fatalf("err = %v, want ErrInvalidName", err)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		ts := newTestService(t)
		ts.repo.upsertErr = errors.New("boom")

		_, err := ts.svc.CreateProject(context.Background(), "valid name")
		if err == nil {
			t.Fatal("expected an error from a failing repo")
		}
	})
}

func TestOpenProject(t *testing.T) {
	t.Parallel()

	ts := newTestService(t)

	id := newULID(t)
	scopeRules := []scope.Rule{{URL: regexp.MustCompile(`example\.com`)}}
	seed := Project{
		ID:   id,
		Name: "seed",
		Settings: Settings{
			ReqLogBypassOutOfScope: true,
			ReqLogOnlyFindInScope:  true,
			ReqLogSearchExpr:       filter.StringLiteral{Value: "reqlog-expr"},
			InterceptRequests:      true,
			InterceptResponses:     true,
			SenderOnlyFindInScope:  true,
			SenderSearchExpr:       filter.StringLiteral{Value: "sender-expr"},
			ScopeRules:             scopeRules,
		},
	}

	if err := ts.repo.UpsertProject(context.Background(), seed); err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	got, err := ts.svc.OpenProject(context.Background(), id)
	if err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}

	if got.ID.Compare(id) != 0 {
		t.Errorf("returned project ID = %v, want %v", got.ID, id)
	}

	if !ts.svc.IsProjectActive(id) {
		t.Error("expected the opened project to be active")
	}

	if ts.reqLogSvc.ActiveProjectID().Compare(id) != 0 {
		t.Error("expected reqlog service active project ID to be updated")
	}

	if !ts.reqLogSvc.BypassOutOfScopeRequests() {
		t.Error("expected reqlog bypass-out-of-scope to be propagated")
	}

	reqLogFilter := ts.reqLogSvc.FindReqsFilter()
	if reqLogFilter.ProjectID.Compare(id) != 0 || !reqLogFilter.OnlyInScope {
		t.Errorf("reqlog find filter not propagated correctly: %+v", reqLogFilter)
	}

	senderFilter := ts.senderSvc.FindReqsFilter()
	if senderFilter.ProjectID.Compare(id) != 0 || !senderFilter.OnlyInScope {
		t.Errorf("sender find filter not propagated correctly: %+v", senderFilter)
	}

	if got := ts.scopeSvc.Rules(); len(got) != 1 || got[0].URL.String() != scopeRules[0].URL.String() {
		t.Errorf("scope rules not propagated correctly: %+v", got)
	}
}

func TestOpenProjectNotFound(t *testing.T) {
	t.Parallel()

	ts := newTestService(t)

	_, err := ts.svc.OpenProject(context.Background(), newULID(t))
	if err == nil {
		t.Fatal("expected an error opening a project that doesn't exist")
	}
}

func TestActiveProject(t *testing.T) {
	t.Parallel()

	ts := newTestService(t)

	_, err := ts.svc.ActiveProject(context.Background())
	if !errors.Is(err, ErrNoProject) {
		t.Fatalf("err = %v, want ErrNoProject before any project is opened", err)
	}

	id := newULID(t)
	if err := ts.repo.UpsertProject(context.Background(), Project{ID: id, Name: "p"}); err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	if _, err := ts.svc.OpenProject(context.Background(), id); err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}

	active, err := ts.svc.ActiveProject(context.Background())
	if err != nil {
		t.Fatalf("ActiveProject failed: %v", err)
	}

	if !active.isActive {
		t.Error("expected the active project to be flagged isActive")
	}

	if active.ID.Compare(id) != 0 {
		t.Errorf("active.ID = %v, want %v", active.ID, id)
	}
}

func TestCloseProject(t *testing.T) {
	t.Parallel()

	t.Run("no project open is a no-op", func(t *testing.T) {
		t.Parallel()

		ts := newTestService(t)

		if err := ts.svc.CloseProject(); err != nil {
			t.Fatalf("CloseProject failed: %v", err)
		}
	})

	t.Run("resets propagated state", func(t *testing.T) {
		t.Parallel()

		ts := newTestService(t)

		id := newULID(t)
		seed := Project{
			ID:   id,
			Name: "p",
			Settings: Settings{
				ReqLogBypassOutOfScope: true,
				ScopeRules:             []scope.Rule{{URL: regexp.MustCompile(`.*`)}},
			},
		}

		if err := ts.repo.UpsertProject(context.Background(), seed); err != nil {
			t.Fatalf("failed to seed project: %v", err)
		}

		if _, err := ts.svc.OpenProject(context.Background(), id); err != nil {
			t.Fatalf("OpenProject failed: %v", err)
		}

		if err := ts.svc.CloseProject(); err != nil {
			t.Fatalf("CloseProject failed: %v", err)
		}

		if ts.svc.IsProjectActive(id) {
			t.Error("expected project to no longer be active")
		}

		if ts.reqLogSvc.ActiveProjectID().Compare(ulid.ULID{}) != 0 {
			t.Error("expected reqlog active project ID to be reset")
		}

		if ts.reqLogSvc.BypassOutOfScopeRequests() {
			t.Error("expected reqlog bypass-out-of-scope to be reset")
		}

		if got := ts.scopeSvc.Rules(); len(got) != 0 {
			t.Errorf("expected scope rules to be cleared, got %d", len(got))
		}

		if _, err := ts.svc.ActiveProject(context.Background()); !errors.Is(err, ErrNoProject) {
			t.Errorf("err = %v, want ErrNoProject after closing", err)
		}
	})
}

func TestDeleteProject(t *testing.T) {
	t.Parallel()

	t.Run("active project cannot be deleted", func(t *testing.T) {
		t.Parallel()

		ts := newTestService(t)

		id := newULID(t)
		if err := ts.repo.UpsertProject(context.Background(), Project{ID: id, Name: "p"}); err != nil {
			t.Fatalf("failed to seed project: %v", err)
		}

		if _, err := ts.svc.OpenProject(context.Background(), id); err != nil {
			t.Fatalf("OpenProject failed: %v", err)
		}

		if err := ts.svc.DeleteProject(context.Background(), id); err == nil {
			t.Fatal("expected an error deleting the active project")
		}
	})

	t.Run("inactive project can be deleted", func(t *testing.T) {
		t.Parallel()

		ts := newTestService(t)

		id := newULID(t)
		if err := ts.repo.UpsertProject(context.Background(), Project{ID: id, Name: "p"}); err != nil {
			t.Fatalf("failed to seed project: %v", err)
		}

		if err := ts.svc.DeleteProject(context.Background(), id); err != nil {
			t.Fatalf("DeleteProject failed: %v", err)
		}

		if _, err := ts.repo.FindProjectByID(context.Background(), id); !errors.Is(err, ErrProjectNotFound) {
			t.Errorf("expected project to be gone from the repo, err = %v", err)
		}
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()

		ts := newTestService(t)
		ts.repo.deleteErr = errors.New("boom")

		if err := ts.svc.DeleteProject(context.Background(), newULID(t)); err == nil {
			t.Fatal("expected an error from a failing repo")
		}
	})
}

func TestSetScopeRules(t *testing.T) {
	t.Parallel()

	t.Run("no active project", func(t *testing.T) {
		t.Parallel()

		ts := newTestService(t)

		err := ts.svc.SetScopeRules(context.Background(), []scope.Rule{{URL: regexp.MustCompile(`.*`)}})
		if !errors.Is(err, ErrNoProject) {
			t.Fatalf("err = %v, want ErrNoProject", err)
		}
	})

	t.Run("updates repo and live scope", func(t *testing.T) {
		t.Parallel()

		ts := newTestService(t)

		id := newULID(t)
		if err := ts.repo.UpsertProject(context.Background(), Project{ID: id, Name: "p"}); err != nil {
			t.Fatalf("failed to seed project: %v", err)
		}

		if _, err := ts.svc.OpenProject(context.Background(), id); err != nil {
			t.Fatalf("OpenProject failed: %v", err)
		}

		rules := []scope.Rule{{URL: regexp.MustCompile(`example\.com`)}}
		if err := ts.svc.SetScopeRules(context.Background(), rules); err != nil {
			t.Fatalf("SetScopeRules failed: %v", err)
		}

		if got := ts.scopeSvc.Rules(); len(got) != 1 {
			t.Fatalf("expected live scope to be updated, got %d rules", len(got))
		}

		stored, err := ts.repo.FindProjectByID(context.Background(), id)
		if err != nil {
			t.Fatalf("FindProjectByID failed: %v", err)
		}

		if len(stored.Settings.ScopeRules) != 1 {
			t.Errorf("expected persisted scope rules, got %d", len(stored.Settings.ScopeRules))
		}
	})
}

func TestSetRequestLogFindFilter(t *testing.T) {
	t.Parallel()

	ts := newTestService(t)

	id := newULID(t)
	if err := ts.repo.UpsertProject(context.Background(), Project{ID: id, Name: "p"}); err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	if _, err := ts.svc.OpenProject(context.Background(), id); err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}

	// A caller-supplied ProjectID must be overridden with the active project's.
	err := ts.svc.SetRequestLogFindFilter(context.Background(), reqlog.FindRequestsFilter{
		ProjectID:   newULID(t),
		OnlyInScope: true,
	})
	if err != nil {
		t.Fatalf("SetRequestLogFindFilter failed: %v", err)
	}

	got := ts.reqLogSvc.FindReqsFilter()
	if got.ProjectID.Compare(id) != 0 {
		t.Errorf("filter.ProjectID = %v, want the active project ID %v", got.ProjectID, id)
	}

	if !got.OnlyInScope {
		t.Error("expected OnlyInScope to propagate")
	}

	stored, err := ts.repo.FindProjectByID(context.Background(), id)
	if err != nil {
		t.Fatalf("FindProjectByID failed: %v", err)
	}

	if !stored.Settings.ReqLogOnlyFindInScope {
		t.Error("expected persisted ReqLogOnlyFindInScope to be true")
	}
}

func TestSetSenderRequestFindFilter(t *testing.T) {
	t.Parallel()

	ts := newTestService(t)

	id := newULID(t)
	if err := ts.repo.UpsertProject(context.Background(), Project{ID: id, Name: "p"}); err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	if _, err := ts.svc.OpenProject(context.Background(), id); err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}

	err := ts.svc.SetSenderRequestFindFilter(context.Background(), sender.FindRequestsFilter{
		ProjectID:   newULID(t),
		OnlyInScope: true,
	})
	if err != nil {
		t.Fatalf("SetSenderRequestFindFilter failed: %v", err)
	}

	got := ts.senderSvc.FindReqsFilter()
	if got.ProjectID.Compare(id) != 0 {
		t.Errorf("filter.ProjectID = %v, want the active project ID %v", got.ProjectID, id)
	}

	stored, err := ts.repo.FindProjectByID(context.Background(), id)
	if err != nil {
		t.Fatalf("FindProjectByID failed: %v", err)
	}

	if !stored.Settings.SenderOnlyFindInScope {
		t.Error("expected persisted SenderOnlyFindInScope to be true")
	}
}

func TestUpdateInterceptSettings(t *testing.T) {
	t.Parallel()

	t.Run("no active project", func(t *testing.T) {
		t.Parallel()

		ts := newTestService(t)

		err := ts.svc.UpdateInterceptSettings(context.Background(), intercept.Settings{RequestsEnabled: true})
		if !errors.Is(err, ErrNoProject) {
			t.Fatalf("err = %v, want ErrNoProject", err)
		}
	})

	t.Run("persists settings", func(t *testing.T) {
		t.Parallel()

		ts := newTestService(t)

		id := newULID(t)
		if err := ts.repo.UpsertProject(context.Background(), Project{ID: id, Name: "p"}); err != nil {
			t.Fatalf("failed to seed project: %v", err)
		}

		if _, err := ts.svc.OpenProject(context.Background(), id); err != nil {
			t.Fatalf("OpenProject failed: %v", err)
		}

		err := ts.svc.UpdateInterceptSettings(context.Background(), intercept.Settings{
			RequestsEnabled:  true,
			ResponsesEnabled: true,
		})
		if err != nil {
			t.Fatalf("UpdateInterceptSettings failed: %v", err)
		}

		stored, err := ts.repo.FindProjectByID(context.Background(), id)
		if err != nil {
			t.Fatalf("FindProjectByID failed: %v", err)
		}

		if !stored.Settings.InterceptRequests || !stored.Settings.InterceptResponses {
			t.Errorf("expected persisted intercept settings to be enabled, got %+v", stored.Settings)
		}
	})
}

func TestProjects(t *testing.T) {
	t.Parallel()

	ts := newTestService(t)

	if err := ts.repo.UpsertProject(context.Background(), Project{ID: newULID(t), Name: "a"}); err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	if err := ts.repo.UpsertProject(context.Background(), Project{ID: newULID(t), Name: "b"}); err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	projects, err := ts.svc.Projects(context.Background())
	if err != nil {
		t.Fatalf("Projects failed: %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}

func TestScope(t *testing.T) {
	t.Parallel()

	ts := newTestService(t)

	if got := ts.svc.Scope(); got != ts.scopeSvc {
		t.Error("expected Scope() to return the configured scope instance")
	}
}

func TestActiveProjectNotFoundInRepo(t *testing.T) {
	t.Parallel()

	ts := newTestService(t)

	id := newULID(t)
	if err := ts.repo.UpsertProject(context.Background(), Project{ID: id, Name: "p"}); err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	if _, err := ts.svc.OpenProject(context.Background(), id); err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}

	// Simulate the active project having been removed from the repo out from
	// under the service.
	ts.repo.findErr = errors.New("boom")

	if _, err := ts.svc.ActiveProject(context.Background()); err == nil {
		t.Fatal("expected an error when the active project can't be loaded")
	}
}

func TestSetScopeRulesRepoError(t *testing.T) {
	t.Parallel()

	ts := newTestService(t)

	id := newULID(t)
	if err := ts.repo.UpsertProject(context.Background(), Project{ID: id, Name: "p"}); err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	if _, err := ts.svc.OpenProject(context.Background(), id); err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}

	ts.repo.upsertErr = errors.New("boom")

	if err := ts.svc.SetScopeRules(context.Background(), nil); err == nil {
		t.Fatal("expected an error from a failing repo")
	}
}

func TestIsProjectActive(t *testing.T) {
	t.Parallel()

	ts := newTestService(t)

	id := newULID(t)
	if ts.svc.IsProjectActive(id) {
		t.Error("expected no project to be active initially")
	}

	if err := ts.repo.UpsertProject(context.Background(), Project{ID: id, Name: "p"}); err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	if _, err := ts.svc.OpenProject(context.Background(), id); err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}

	if !ts.svc.IsProjectActive(id) {
		t.Error("expected the opened project to be active")
	}

	if ts.svc.IsProjectActive(newULID(t)) {
		t.Error("expected a different project ID to not be active")
	}
}
