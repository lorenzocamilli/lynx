package bolt_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/oklog/ulid"
	"go.etcd.io/bbolt"

	"github.com/lorenzocamilli/lynx/pkg/db/bolt"
	"github.com/lorenzocamilli/lynx/pkg/proj"
	"github.com/lorenzocamilli/lynx/pkg/reqlog"
)

func TestFindRequestLogs(t *testing.T) {
	t.Parallel()

	t.Run("without project ID in filter", func(t *testing.T) {
		t.Parallel()

		path := t.TempDir() + "bolt.db"
		boltDB, err := bbolt.Open(path, 0o600, nil)
		if err != nil {
			t.Fatalf("failed to open bolt database: %v", err)
		}

		db, err := bolt.DatabaseFromBoltDB(boltDB)
		if err != nil {
			t.Fatalf("failed to create database: %v", err)
		}
		defer db.Close()

		filter := reqlog.FindRequestsFilter{}

		_, err = db.FindRequestLogs(context.Background(), filter, nil)
		if !errors.Is(err, reqlog.ErrProjectIDMustBeSet) {
			t.Fatalf("expected `reqlog.ErrProjectIDMustBeSet`, got: %v", err)
		}
	})

	t.Run("returns request logs and related response logs", func(t *testing.T) {
		t.Parallel()

		path := t.TempDir() + "bolt.db"
		boltDB, err := bbolt.Open(path, 0o600, nil)
		if err != nil {
			t.Fatalf("failed to open bolt database: %v", err)
		}

		db, err := bolt.DatabaseFromBoltDB(boltDB)
		if err != nil {
			t.Fatalf("failed to create database: %v", err)
		}
		defer db.Close()

		projectID := ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)

		err = db.UpsertProject(context.Background(), proj.Project{
			ID: projectID,
		})
		if err != nil {
			t.Fatalf("unexpected error upserting project: %v", err)
		}

		fixtures := []reqlog.RequestLog{
			{
				ID:        ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy),
				ProjectID: projectID,
				URL:       mustParseURL(t, "https://example.com/foobar"),
				Method:    http.MethodPost,
				Proto:     "HTTP/1.1",
				Header: http.Header{
					"X-Foo": []string{"baz"},
				},
				Body: []byte("foo"),
				Response: &reqlog.ResponseLog{
					Proto:      "HTTP/1.1",
					Status:     "200 OK",
					StatusCode: 200,
					Header: http.Header{
						"X-Yolo": []string{"swag"},
					},
					Body: []byte("bar"),
				},
			},
			{
				ID:        ulid.MustNew(ulid.Timestamp(time.Now())+100, ulidEntropy),
				ProjectID: projectID,
				URL:       mustParseURL(t, "https://example.com/foo?bar=baz"),
				Method:    http.MethodGet,
				Proto:     "HTTP/1.1",
				Header: http.Header{
					"X-Foo": []string{"baz"},
				},
			},
		}

		// Store fixtures.
		for _, reqLog := range fixtures {
			err = db.StoreRequestLog(context.Background(), reqLog)
			if err != nil {
				t.Fatalf("unexpected error creating request log fixture: %v", err)
			}
		}

		filter := reqlog.FindRequestsFilter{
			ProjectID: projectID,
		}

		got, err := db.FindRequestLogs(context.Background(), filter, nil)
		if err != nil {
			t.Fatalf("unexpected error finding request logs: %v", err)
		}

		// We expect the found request logs are *reversed*, e.g. newest first.
		exp := make([]reqlog.RequestLog, len(fixtures))
		for i, j := 0, len(fixtures)-1; i < j; i, j = i+1, j-1 {
			exp[i], exp[j] = fixtures[j], fixtures[i]
		}

		if diff := cmp.Diff(exp, got); diff != "" {
			t.Fatalf("request logs not equal (-exp, +got):\n%v", diff)
		}
	})
}

func TestDeleteRequestLog(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "bolt.db"
	boltDB, err := bbolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatalf("failed to open bolt database: %v", err)
	}

	db, err := bolt.DatabaseFromBoltDB(boltDB)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	projectID := ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)

	err = db.UpsertProject(context.Background(), proj.Project{
		ID: projectID,
	})
	if err != nil {
		t.Fatalf("unexpected error upserting project: %v", err)
	}

	fixture := reqlog.RequestLog{
		ID:        ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy),
		ProjectID: projectID,
		URL:       mustParseURL(t, "https://example.com/delete-test"),
		Method:    http.MethodGet,
		Proto:     "HTTP/1.1",
	}

	err = db.StoreRequestLog(context.Background(), fixture)
	if err != nil {
		t.Fatalf("unexpected error storing request log: %v", err)
	}

	err = db.DeleteRequestLog(context.Background(), projectID, fixture.ID)
	if err != nil {
		t.Fatalf("unexpected error deleting request log: %v", err)
	}

	_, err = db.FindRequestLogByID(context.Background(), projectID, fixture.ID)
	if !errors.Is(err, reqlog.ErrRequestNotFound) {
		t.Fatalf("expected reqlog.ErrRequestNotFound after delete, got: %v", err)
	}

	// BoltDB Delete is idempotent — deleting a non-existent key returns nil.
	err = db.DeleteRequestLog(context.Background(), projectID, fixture.ID)
	if err != nil {
		t.Fatalf("expected nil on second delete, got: %v", err)
	}
}

func TestCountRequestLogs(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "bolt.db"
	boltDB, err := bbolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatalf("failed to open bolt database: %v", err)
	}

	db, err := bolt.DatabaseFromBoltDB(boltDB)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	projectID := ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)

	err = db.UpsertProject(context.Background(), proj.Project{
		ID: projectID,
	})
	if err != nil {
		t.Fatalf("unexpected error upserting project: %v", err)
	}

	fixtures := []reqlog.RequestLog{
		{
			ID:        ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy),
			ProjectID: projectID,
			URL:       mustParseURL(t, "https://example.com/count-1"),
			Method:    http.MethodGet,
			Proto:     "HTTP/1.1",
		},
		{
			ID:        ulid.MustNew(ulid.Timestamp(time.Now())+100, ulidEntropy),
			ProjectID: projectID,
			URL:       mustParseURL(t, "https://example.com/count-2"),
			Method:    http.MethodGet,
			Proto:     "HTTP/1.1",
		},
		{
			ID:        ulid.MustNew(ulid.Timestamp(time.Now())+200, ulidEntropy),
			ProjectID: projectID,
			URL:       mustParseURL(t, "https://example.com/count-3"),
			Method:    http.MethodGet,
			Proto:     "HTTP/1.1",
		},
	}

	for _, reqLog := range fixtures {
		err = db.StoreRequestLog(context.Background(), reqLog)
		if err != nil {
			t.Fatalf("unexpected error storing request log: %v", err)
		}
	}

	count, err := db.CountRequestLogs(context.Background(), projectID)
	if err != nil {
		t.Fatalf("unexpected error counting request logs: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}

	err = db.DeleteRequestLog(context.Background(), projectID, fixtures[0].ID)
	if err != nil {
		t.Fatalf("unexpected error deleting request log: %v", err)
	}

	count, err = db.CountRequestLogs(context.Background(), projectID)
	if err != nil {
		t.Fatalf("unexpected error counting request logs after delete: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2 after delete, got %d", count)
	}
}

func mustParseURL(t *testing.T, s string) *url.URL {
	t.Helper()

	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}

	return u
}
