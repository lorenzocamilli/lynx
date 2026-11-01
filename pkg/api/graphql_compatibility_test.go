package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid"

	"github.com/lorenzocamilli/lynx/pkg/db/bolt"
	"github.com/lorenzocamilli/lynx/pkg/proj"
	"github.com/lorenzocamilli/lynx/pkg/proxy"
	"github.com/lorenzocamilli/lynx/pkg/proxy/intercept"
	"github.com/lorenzocamilli/lynx/pkg/reqlog"
	"github.com/lorenzocamilli/lynx/pkg/scope"
	"github.com/lorenzocamilli/lynx/pkg/sender"
)

// GraphQL scalar URLs are strings on the wire, not encoding/json url.URL objects.
type graphqlSenderRequest struct {
	SenderRequest
	URL string `json:"url"`
}

type graphqlHTTPRequest struct {
	HTTPRequest
	URL string `json:"url"`
}

type graphqlTestResult struct {
	StatusCode int             `json:"-"`
	Data       json.RawMessage `json:"data"`
	Errors     []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

func graphqlPOST(t *testing.T, handler http.Handler, payload map[string]any) graphqlTestResult {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/graphql/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code >= 500 {
		t.Fatalf("GraphQL server failure: %d %s", rec.Code, rec.Body)
	}
	var result graphqlTestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid GraphQL response: %s, %v", rec.Body, err)
	}
	result.StatusCode = rec.Code
	return result
}

func graphqlData(t *testing.T, handler http.Handler, query string, variables map[string]any, target any) {
	t.Helper()
	result := graphqlPOST(t, handler, map[string]any{"query": query, "variables": variables})
	if result.StatusCode != http.StatusOK || len(result.Errors) != 0 {
		t.Fatalf("GraphQL response: status=%d errors=%+v", result.StatusCode, result.Errors)
	}
	if err := json.Unmarshal(result.Data, target); err != nil {
		t.Fatal(err)
	}
}

func compatibilityServices(t *testing.T, path string) (*bolt.Database, *Resolver) {
	t.Helper()
	db, err := bolt.OpenDatabase(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	scopeSvc := &scope.Scope{}
	logs := reqlog.NewService(reqlog.Config{Repository: db, Scope: scopeSvc})
	interceptor := intercept.NewService(intercept.Config{})
	senderSvc := sender.NewService(sender.Config{Repository: db, ReqLogService: logs, Scope: scopeSvc})
	projects, err := proj.NewService(proj.Config{
		Repository: db, Scope: scopeSvc, ReqLogService: logs,
		InterceptService: interceptor, SenderService: senderSvc,
	})
	if err != nil {
		t.Fatal(err)
	}
	return db, &Resolver{ProjectService: projects, RequestLogService: logs, InterceptService: interceptor, SenderService: senderSvc}
}

// Exercise the actual HTTP decoder, validator, generated execution, custom
// scalars/enums and resolvers, then reopen Bolt and query the persisted models.
func TestGraphQLProjectAndStoredModels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lynx.db")
	db, resolver := compatibilityServices(t, path)
	handler := HTTPHandler(resolver, "/api/graphql/")
	var created struct{ CreateProject Project }
	graphqlData(t, handler, `mutation Create($name:String!) { createProject(name:$name) { id name isActive } }`, map[string]any{"name": "GraphQL compatibility"}, &created)
	projectID := created.CreateProject.ID
	if projectID == (ulid.ULID{}) || created.CreateProject.Name != "GraphQL compatibility" {
		t.Fatalf("unexpected project: %+v", created)
	}
	var opened struct{ OpenProject Project }
	graphqlData(t, handler, `mutation($id:ID!) { openProject(id:$id) { id name isActive } }`, map[string]any{"id": projectID.String()}, &opened)
	if !opened.OpenProject.IsActive {
		t.Fatal("project did not open")
	}
	var settings struct{ UpdateInterceptSettings InterceptSettings }
	graphqlData(t, handler, `mutation($input:UpdateInterceptSettingsInput!) { updateInterceptSettings(input:$input) { requestsEnabled responsesEnabled requestFilter responseFilter } }`, map[string]any{
		"input": map[string]any{"requestsEnabled": true, "responsesEnabled": false, "requestFilter": "method = POST", "responseFilter": nil},
	}, &settings)
	var rules struct{ SetScope []ScopeRule }
	graphqlData(t, handler, `mutation($rules:[ScopeRuleInput!]!) { setScope(scope:$rules) { url header { key value } body } }`, map[string]any{
		"rules": []any{map[string]any{"url": `^https://example\.com/`, "body": "payload"}},
	}, &rules)

	logID := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	requestURL, err := url.Parse("https://example.com/a%2Fb?q=a%26b#fragment")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.StoreRequestLog(context.Background(), reqlog.RequestLog{
		ID: logID, ProjectID: projectID, URL: requestURL, Method: "POST", Proto: "HTTP/1.1",
		Header: http.Header{"X-Test": {"one", "two"}}, Body: []byte("stored payload"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.StoreResponseLog(context.Background(), projectID, logID, reqlog.ResponseLog{
		Proto: "HTTP/1.1", StatusCode: 202, Status: "202 Accepted", Body: []byte("stored response"),
		Header: http.Header{"Content-Type": {"text/plain"}}, Duration: 12 * time.Millisecond, Size: 15,
	}); err != nil {
		t.Fatal(err)
	}
	var cloned struct{ CreateSenderRequestFromHTTPRequestLog graphqlSenderRequest }
	graphqlData(t, handler, `mutation($id:ID!) { createSenderRequestFromHttpRequestLog(id:$id) { id sourceRequestLogID url method proto body headers { key value } response { statusCode } } }`, map[string]any{"id": logID.String()}, &cloned)
	clone := cloned.CreateSenderRequestFromHTTPRequestLog
	if clone.ID == (ulid.ULID{}) || clone.SourceRequestLogID == nil || *clone.SourceRequestLogID != logID || clone.Body == nil || *clone.Body != "stored payload" || clone.Method != HTTPMethodPost || clone.Response != nil {
		t.Fatalf("clone changed: %+v", clone)
	}
	var edited struct{ CreateOrUpdateSenderRequest graphqlSenderRequest }
	graphqlData(t, handler, `mutation($input:SenderRequestInput!) { createOrUpdateSenderRequest(request:$input) { id url method proto body } }`, map[string]any{
		"input": map[string]any{"id": clone.ID.String(), "url": requestURL.String(), "method": "PATCH", "proto": "HTTP11", "body": "edited payload"},
	}, &edited)
	if edited.CreateOrUpdateSenderRequest.Proto != HTTPProtocolHTTP11 {
		t.Fatalf("protocol enum changed: %+v", edited)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	_, resolver = compatibilityServices(t, path)
	handler = HTTPHandler(resolver, "/api/graphql/")
	graphqlData(t, handler, `mutation($id:ID!) { openProject(id:$id) { id isActive } }`, map[string]any{"id": projectID.String()}, &opened)
	var stored struct {
		ActiveProject        *Project
		Scope                []ScopeRule
		HTTPRequestLog       *HTTPRequestLog
		SenderRequest        *graphqlSenderRequest
		HTTPRequestLogsCount int
	}
	graphqlData(t, handler, `query Models($log:ID!, $sender:ID!) {
		activeProject { id name settings { intercept { requestsEnabled responsesEnabled requestFilter responseFilter } } }
		scope { url body }
		httpRequestLogsCount
		httpRequestLog(id:$log) { id url method proto body timestamp headers { key value } response { id proto statusCode statusReason body durationMs size } }
		senderRequest(id:$sender) { id sourceRequestLogID url method proto body response { statusCode } }
	}`, map[string]any{"log": logID.String(), "sender": clone.ID.String()}, &stored)
	if stored.ActiveProject == nil || stored.ActiveProject.ID != projectID || stored.ActiveProject.Settings.Intercept.RequestFilter == nil || *stored.ActiveProject.Settings.Intercept.RequestFilter != `("method" = "POST")` || !stored.ActiveProject.Settings.Intercept.RequestsEnabled {
		t.Fatalf("persisted project settings changed: %+v", stored.ActiveProject)
	}
	if len(stored.Scope) != 1 || stored.Scope[0].URL == nil || *stored.Scope[0].URL != `^https://example\.com/` {
		t.Fatalf("persisted regex changed: %+v", stored.Scope)
	}
	log := stored.HTTPRequestLog
	if stored.HTTPRequestLogsCount != 1 || log == nil || log.ID != logID || log.URL != requestURL.String() || log.Body == nil || *log.Body != "stored payload" || !log.Timestamp.Equal(ulid.Time(logID.Time())) || log.Response == nil || log.Response.Proto != HTTPProtocolHTTP11 || log.Response.StatusCode != 202 || log.Response.DurationMs != 12 || log.Response.Size != 15 || log.Response.Body == nil || *log.Response.Body != "stored response" {
		t.Fatalf("persisted log changed: %+v", log)
	}
	request := stored.SenderRequest
	if request == nil || request.ID != clone.ID || request.URL != requestURL.String() || request.Method != HTTPMethodPatch || request.Proto != HTTPProtocolHTTP11 || request.Body == nil || *request.Body != "edited payload" || request.Response != nil {
		t.Fatalf("persisted Sender request changed: %+v", request)
	}
}

func TestGraphQLValidationAndPersistedQueries(t *testing.T) {
	handler := HTTPHandler(&Resolver{}, "/api/graphql/")
	for _, tc := range []struct {
		name, query string
		variables   map[string]any
	}{
		{name: "malformed-directive", query: `query { __typename @ }`},
		{name: "unknown-field", query: `{ fieldThatDoesNotExist }`},
		{name: "invalid-variable-type", query: `query($id:ID!) { senderRequest(id:$id) { id } }`, variables: map[string]any{"id": []string{"not-an-id"}}},
		{name: "invalid-id", query: `query($id:ID!) { senderRequest(id:$id) { id } }`, variables: map[string]any{"id": "invalid"}},
		{name: "invalid-enum", query: `mutation($input:SenderRequestInput!) { createOrUpdateSenderRequest(request:$input) { id } }`, variables: map[string]any{"input": map[string]any{"url": "https://example.com", "method": "INVALID"}}},
		{name: "excessive-nesting", query: strings.Repeat("{ a ", 1002) + strings.Repeat("}", 1002)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := graphqlPOST(t, handler, map[string]any{"query": tc.query, "variables": tc.variables})
			if len(result.Errors) == 0 {
				t.Fatal("invalid operation was not rejected")
			}
			for _, err := range result.Errors {
				if strings.Contains(strings.ToLower(err.Message), "internal") {
					t.Fatalf("validation reached an unconfigured resolver: %+v", result.Errors)
				}
			}
		})
	}
	// Named operations, fragments, variable defaults and standard directives
	// must still run through generated execution and introspection.
	result := graphqlPOST(t, handler, map[string]any{
		"query":         `query Other { __typename } query Selected($show:Boolean! = true) { ...Root } fragment Root on Query { __typename @include(if:$show) }`,
		"operationName": "Selected",
	})
	if len(result.Errors) != 0 || string(result.Data) != `{"__typename":"Query"}` {
		t.Fatalf("named operation/default/directive failed: %+v", result)
	}
	var schema struct {
		Schema struct{ QueryType struct{ Name string } } `json:"__schema"`
	}
	graphqlData(t, handler, `{ __schema { queryType { name } } }`, nil, &schema)
	if schema.Schema.QueryType.Name != "Query" {
		t.Fatal("introspection compatibility changed")
	}
	query := `{ __typename }`
	extensions := map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": fmt.Sprintf("%x", sha256.Sum256([]byte(query)))}}
	result = graphqlPOST(t, handler, map[string]any{"extensions": extensions})
	if len(result.Errors) != 1 || result.Errors[0].Extensions["code"] != "PERSISTED_QUERY_NOT_FOUND" {
		t.Fatalf("persisted-query cache miss changed: %+v", result)
	}
	for _, payload := range []map[string]any{{"query": query, "extensions": extensions}, {"extensions": extensions}} {
		result = graphqlPOST(t, handler, payload)
		if len(result.Errors) != 0 || string(result.Data) != `{"__typename":"Query"}` {
			t.Fatalf("persisted-query registration/replay failed: %+v", result)
		}
	}
}

func TestGraphQLInterceptQueryAndMutation(t *testing.T) {
	svc := intercept.NewService(intercept.Config{RequestsEnabled: true})
	handler := HTTPHandler(&Resolver{InterceptService: svc}, "/api/graphql/")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	req := httptest.NewRequest(http.MethodPost, "https://example.com/original", strings.NewReader("original body"))
	req = req.WithContext(proxy.WithRequestID(ctx, id))
	done := make(chan *http.Request, 1)
	go func() {
		forwarded, err := svc.InterceptRequest(req.Context(), req)
		if err != nil && ctx.Err() == nil {
			t.Error(err)
		}
		done <- forwarded
	}()
	deadline := time.Now().Add(2 * time.Second)
	for len(svc.Items()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("request was not intercepted")
		}
		time.Sleep(time.Millisecond)
	}
	var pending struct{ InterceptedRequests []graphqlHTTPRequest }
	graphqlData(t, handler, `{ interceptedRequests { id url method proto body response { statusCode } } }`, nil, &pending)
	if len(pending.InterceptedRequests) != 1 || pending.InterceptedRequests[0].ID != id || pending.InterceptedRequests[0].Proto != HTTPProtocolHTTP11 {
		t.Fatalf("intercepted query changed: %+v", pending)
	}
	var modified struct{ ModifyRequest ModifyRequestResult }
	graphqlData(t, handler, `mutation($request:ModifyRequestInput!) { modifyRequest(request:$request) { success } }`, map[string]any{
		"request": map[string]any{"id": id.String(), "url": "https://example.com/edited", "method": "PATCH", "proto": "HTTP11", "body": "edited body"},
	}, &modified)
	select {
	case forwarded := <-done:
		if forwarded == nil {
			t.Fatal("request was cancelled")
		}
		body, err := io.ReadAll(forwarded.Body)
		if !modified.ModifyRequest.Success || err != nil || string(body) != "edited body" || forwarded.Method != http.MethodPatch || forwarded.URL.Path != "/edited" {
			t.Fatalf("HTTP mutation did not preserve edits: %v, %q, %v", forwarded, body, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP mutation did not unblock forwarding")
	}
}

func TestGraphQLLegacyDatabase(t *testing.T) {
	compressed, err := os.Open("testdata/pre-gqlgen-upgrade.db.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	reader, err := gzip.NewReader(compressed)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	path := filepath.Join(t.TempDir(), "legacy.db")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("extract fixture: copy=%v, close=%v", copyErr, closeErr)
	}
	_, resolver := compatibilityServices(t, path)
	handler := HTTPHandler(resolver, "/api/graphql/")
	var opened struct{ OpenProject Project }
	graphqlData(t, handler, `mutation { openProject(id:"01ARZ3NDEKTSV4RRFFQ69G5FAW") { id name settings { intercept { requestsEnabled requestFilter } } } }`, nil, &opened)
	project := opened.OpenProject
	if project.Name != "Legacy compatibility" || project.Settings.Intercept.RequestFilter == nil || *project.Settings.Intercept.RequestFilter != `("method" = "POST")` || !project.Settings.Intercept.RequestsEnabled {
		t.Fatalf("legacy project/filter failed: %+v", project)
	}
	var data struct {
		HTTPRequestLogs []HTTPRequestLog
		SenderRequests  []graphqlSenderRequest
		Scope           []ScopeRule
	}
	graphqlData(t, handler, `{
		httpRequestLogs { id url method body timestamp headers { key value } response { proto statusCode body durationMs size } }
		senderRequests { id sourceRequestLogID url method proto body response { statusCode body } }
		scope { url }
	}`, nil, &data)
	if len(data.HTTPRequestLogs) != 1 || len(data.SenderRequests) != 1 || len(data.Scope) != 1 {
		t.Fatalf("legacy records missing: %+v", data)
	}
	log := data.HTTPRequestLogs[0]
	if log.ID.String() != "01ARZ3NDEKTSV4RRFFQ69G5FAV" || log.Body == nil || *log.Body != "legacy request" || log.URL != "https://example.com/a%2Fb?q=a%26b#fragment" || len(log.Headers) != 2 || log.Response == nil || log.Response.Proto != HTTPProtocolHTTP11 || log.Response.StatusCode != 202 || log.Response.DurationMs != 12 || log.Response.Size != 15 || log.Response.Body == nil || *log.Response.Body != "legacy response" {
		t.Fatalf("legacy request/response changed: %+v", log)
	}
	request := data.SenderRequests[0]
	if request.ID.String() != "01ARZ3NDEKTSV4RRFFQ69G5FAX" || request.SourceRequestLogID == nil || *request.SourceRequestLogID != log.ID || request.Method != HTTPMethodPost || request.Proto != HTTPProtocolHTTP20 || request.Body == nil || *request.Body != "legacy sender" || request.Response == nil || request.Response.Body == nil || *request.Response.Body != "legacy response" {
		t.Fatalf("legacy Sender request changed: %+v", request)
	}
	if data.Scope[0].URL == nil || *data.Scope[0].URL != `^https://example\.com/` {
		t.Fatalf("legacy scope regex changed: %+v", data.Scope)
	}
}
