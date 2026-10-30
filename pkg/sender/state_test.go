package sender_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/oklog/ulid"

	"github.com/lorenzocamilli/lynx/pkg/reqlog"
	"github.com/lorenzocamilli/lynx/pkg/scope"
	"github.com/lorenzocamilli/lynx/pkg/sender"
)

type stateRepository struct {
	sender.Repository
	find   func(ulid.ULID, ulid.ULID) (sender.Request, error)
	store  func(sender.Request) error
	search func(sender.FindRequestsFilter, *scope.Scope) ([]sender.Request, error)
	delete func(ulid.ULID, ulid.ULID) error
}

func (r stateRepository) FindSenderRequestByID(_ context.Context, projectID, id ulid.ULID) (sender.Request, error) {
	return r.find(projectID, id)
}

func (r stateRepository) StoreSenderRequest(_ context.Context, req sender.Request) error {
	return r.store(req)
}

func (r stateRepository) FindSenderRequests(_ context.Context, filter sender.FindRequestsFilter, rules *scope.Scope) ([]sender.Request, error) {
	return r.search(filter, rules)
}

func (r stateRepository) DeleteSenderRequest(_ context.Context, projectID, id ulid.ULID) error {
	return r.delete(projectID, id)
}

type logLookupRepository struct {
	reqlog.Repository
	find func(ulid.ULID) (reqlog.RequestLog, error)
}

func (r logLookupRepository) FindRequestLogByID(_ context.Context, projectID, _ ulid.ULID) (reqlog.RequestLog, error) {
	return r.find(projectID)
}

func TestCloneUsesOneProjectForLookupAndStorage(t *testing.T) {
	for _, switchDuringLookup := range []bool{false, true} {
		name := "different active log project"
		if switchDuringLookup {
			name = "switch during lookup"
		}
		t.Run(name, func(t *testing.T) {
			a, b := ulid.ULID{1}, ulid.ULID{2}
			var svc *sender.Service
			var lookupProject, storedProject ulid.ULID
			logs := reqlog.NewService(reqlog.Config{
				ActiveProjectID: b,
				Repository: logLookupRepository{find: func(projectID ulid.ULID) (reqlog.RequestLog, error) {
					lookupProject = projectID
					if switchDuringLookup {
						svc.SetActiveProjectID(b)
					}
					return reqlog.RequestLog{ProjectID: projectID, URL: exampleURL}, nil
				}},
			})
			svc = sender.NewService(sender.Config{
				ReqLogService: logs,
				Repository: stateRepository{store: func(req sender.Request) error {
					storedProject = req.ProjectID
					return nil
				}},
			})
			svc.SetActiveProjectID(a)
			if _, err := svc.CloneFromRequestLog(context.Background(), ulid.ULID{3}); err != nil {
				t.Fatal(err)
			}
			if lookupProject != a || storedProject != a {
				t.Fatalf("lookup project = %v, stored project = %v; both must remain %v", lookupProject, storedProject, a)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestSendDoesNotHoldStateLockDuringHTTPOrRetargetResponse(t *testing.T) {
	a, b := ulid.ULID{1}, ulid.ULID{2}
	var svc *sender.Service
	var stored sender.Request
	svc = sender.NewService(sender.Config{
		Repository: stateRepository{
			find: func(projectID, id ulid.ULID) (sender.Request, error) {
				return sender.Request{ID: id, ProjectID: projectID, URL: exampleURL, Proto: sender.HTTPProto11}, nil
			},
			store: func(req sender.Request) error { stored = req; return nil },
		},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			// Reenter settings while the send is in flight. Holding a state
			// lock for network I/O would deadlock here.
			svc.SetActiveProjectID(b)
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
		})},
	})
	svc.SetActiveProjectID(a)
	if _, err := svc.SendRequest(context.Background(), ulid.ULID{3}); err != nil {
		t.Fatal(err)
	}
	if stored.ProjectID != a || stored.Response == nil || string(stored.Response.Body) != "ok" {
		t.Fatalf("in-flight response was not preserved in its original project: %+v", stored)
	}
}

func TestConcurrentSenderStateAccess(t *testing.T) {
	svc := sender.NewService(sender.Config{Repository: stateRepository{
		find: func(projectID, id ulid.ULID) (sender.Request, error) {
			return sender.Request{ID: id, ProjectID: projectID}, nil
		},
		store:  func(sender.Request) error { return nil },
		search: func(sender.FindRequestsFilter, *scope.Scope) ([]sender.Request, error) { return nil, nil },
		delete: func(ulid.ULID, ulid.ULID) error { return nil },
	}})
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				id := ulid.ULID{byte(i % 2)}
				svc.SetActiveProjectID(id)
				svc.SetFindReqsFilter(sender.FindRequestsFilter{ProjectID: id})
				_ = svc.FindReqsFilter()
				_, _ = svc.FindRequestByID(context.Background(), ulid.ULID{3})
				_, _ = svc.FindRequests(context.Background(), 10, 0)
				_, _ = svc.CreateOrUpdateRequest(context.Background(), sender.Request{URL: exampleURL})
				_ = svc.DeleteRequest(context.Background(), ulid.ULID{3})
			}
		}()
	}
	wg.Wait()
}
