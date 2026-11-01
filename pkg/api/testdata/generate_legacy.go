// Run against the baseline checkout documented in README.md, not the upgraded tree.
package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/oklog/ulid"

	"github.com/lorenzocamilli/lynx/pkg/db/bolt"
	"github.com/lorenzocamilli/lynx/pkg/filter"
	"github.com/lorenzocamilli/lynx/pkg/proj"
	"github.com/lorenzocamilli/lynx/pkg/reqlog"
	"github.com/lorenzocamilli/lynx/pkg/scope"
	"github.com/lorenzocamilli/lynx/pkg/sender"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: go run generate_legacy.go /path/to/new/legacy.db")
	}
	db, err := bolt.OpenDatabase(os.Args[1], nil)
	must(err)
	ctx := context.Background()
	projectID := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAW")
	logID := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	senderID := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAX")
	expr, err := filter.ParseQuery("method = POST")
	must(err)
	must(db.UpsertProject(ctx, proj.Project{
		ID: projectID, Name: "Legacy compatibility",
		Settings: proj.Settings{
			InterceptRequests: true, InterceptRequestFilter: expr,
			ScopeRules: []scope.Rule{{URL: regexp.MustCompile(`^https://example\.com/`)}},
		},
	}))
	u, err := url.Parse("https://example.com/a%2Fb?q=a%26b#fragment")
	must(err)
	must(db.StoreRequestLog(ctx, reqlog.RequestLog{
		ID: logID, ProjectID: projectID, URL: u, Method: "POST", Proto: "HTTP/1.1",
		Header: http.Header{"X-Test": {"one", "two"}}, Body: []byte("legacy request"),
	}))
	response := reqlog.ResponseLog{
		Proto: "HTTP/1.1", StatusCode: 202, Status: "202 Accepted", Body: []byte("legacy response"),
		Header: http.Header{"Content-Type": {"text/plain"}}, Duration: 12 * time.Millisecond, Size: 15,
	}
	must(db.StoreResponseLog(ctx, projectID, logID, response))
	must(db.StoreSenderRequest(ctx, sender.Request{
		ID: senderID, ProjectID: projectID, SourceRequestLogID: logID,
		URL: u, Method: "POST", Proto: "HTTP/2.0", Body: []byte("legacy sender"),
		Header: http.Header{"X-Test": {"sender"}}, Response: &response,
	}))
	must(db.Close())
}
