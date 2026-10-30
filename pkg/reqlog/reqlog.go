package reqlog

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/oklog/ulid"

	"github.com/lorenzocamilli/lynx/pkg/filter"
	"github.com/lorenzocamilli/lynx/pkg/log"
	"github.com/lorenzocamilli/lynx/pkg/proxy"
	"github.com/lorenzocamilli/lynx/pkg/scope"
	"github.com/lorenzocamilli/lynx/pkg/sse"
)

type contextKey int

const (
	LogBypassedKey contextKey = iota
	ReqLogIDKey
	projectIDKey
)

var (
	ErrRequestNotFound    = errors.New("reqlog: request not found")
	ErrProjectIDMustBeSet = errors.New("reqlog: project ID must be set")
)

type RequestLog struct {
	ID        ulid.ULID
	ProjectID ulid.ULID

	URL    *url.URL
	Method string
	Proto  string
	Header http.Header
	Body   []byte

	Response *ResponseLog
}

type ResponseLog struct {
	Proto      string
	StatusCode int
	Status     string
	Header     http.Header
	Body       []byte

	// Duration is the upstream round-trip time: the interval between the
	// request being sent and the response arriving, excluding any time spent
	// blocked in the intercept UI. Zero when unknown (e.g. decoded from
	// storage predating this field).
	Duration time.Duration
	// Size is the length in bytes of Body, as read off the wire (i.e. after
	// gunzip, since Body always holds the decompressed content).
	Size int64
}

type Service struct {
	projectMu                *sync.RWMutex
	mu                       sync.RWMutex
	bypassOutOfScopeRequests bool
	findReqsFilter           FindRequestsFilter
	activeProjectID          ulid.ULID
	scope                    *scope.Scope
	repo                     Repository
	logger                   log.Logger
	broadcaster              *sse.Broadcaster
	maxBodyBytes             int64
	redactHeaders            []string
}

type FindRequestsFilter struct {
	ProjectID   ulid.ULID
	OnlyInScope bool
	SearchExpr  filter.Expression
	Limit       int
	Offset      int
}

type Config struct {
	ActiveProjectID ulid.ULID
	Scope           *scope.Scope
	Repository      Repository
	Logger          log.Logger
	Broadcaster     *sse.Broadcaster
	MaxBodyBytes    int64
	RedactHeaders   []string
}

func NewService(cfg Config) *Service {
	maxBody := cfg.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = 10 * 1024 * 1024
	}
	s := &Service{
		projectMu:       &sync.RWMutex{},
		findReqsFilter:  FindRequestsFilter{ProjectID: cfg.ActiveProjectID},
		activeProjectID: cfg.ActiveProjectID,
		repo:            cfg.Repository,
		scope:           cfg.Scope,
		logger:          cfg.Logger,
		broadcaster:     cfg.Broadcaster,
		maxBodyBytes:    maxBody,
		redactHeaders:   cfg.RedactHeaders,
	}

	if s.logger == nil {
		s.logger = log.NewNopLogger()
	}

	return s
}

func (svc *Service) emit(t sse.EventType) {
	if svc.broadcaster != nil {
		svc.broadcaster.Broadcast(sse.Event{Type: t})
	}
}

// SetProjectStateMutex binds state snapshots to the project coordinator.
// Call only during setup, before concurrent use. Setters use mu only because
// the coordinator holds projectMu while applying a complete project update.
func (svc *Service) SetProjectStateMutex(mu *sync.RWMutex) {
	svc.projectMu = mu
}

type projectState struct {
	id               ulid.ULID
	filter           FindRequestsFilter
	bypassOutOfScope bool
	scope            *scope.Scope
}

func (svc *Service) projectState() projectState {
	svc.projectMu.RLock()
	defer svc.projectMu.RUnlock()
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return projectState{svc.activeProjectID, svc.findReqsFilter, svc.bypassOutOfScopeRequests, svc.scope.Snapshot()}
}

func (svc *Service) FindRequests(ctx context.Context, limit, offset int) ([]RequestLog, error) {
	state := svc.projectState()
	filter := state.filter
	filter.Limit = limit
	filter.Offset = offset
	return svc.repo.FindRequestLogs(ctx, filter, state.scope)
}

func (svc *Service) FindRequestLogByID(ctx context.Context, id ulid.ULID) (RequestLog, error) {
	return svc.FindRequestLogByProjectID(ctx, svc.ActiveProjectID(), id)
}

// FindRequestLogByProjectID reads from an explicit operation snapshot rather
// than consulting the active project again during a cross-service operation.
func (svc *Service) FindRequestLogByProjectID(ctx context.Context, projectID, id ulid.ULID) (RequestLog, error) {
	return svc.repo.FindRequestLogByID(ctx, projectID, id)
}

func (svc *Service) CountRequests(ctx context.Context) (int, error) {
	return svc.repo.CountRequestLogs(ctx, svc.ActiveProjectID())
}

func (svc *Service) ClearRequests(ctx context.Context, projectID ulid.ULID) error {
	return svc.repo.ClearRequestLogs(ctx, projectID)
}

func (svc *Service) DeleteRequest(ctx context.Context, projectID, id ulid.ULID) error {
	return svc.repo.DeleteRequestLog(ctx, projectID, id)
}

// captureBody reads only the logged prefix, then replays it before the unread
// remainder. Closing the replacement must still close the original stream.
func captureBody(body io.ReadCloser, limit int64) ([]byte, io.ReadCloser, error) {
	prefix, err := io.ReadAll(io.LimitReader(body, limit))
	return prefix, struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(prefix), body), body}, err
}

func (svc *Service) RequestModifier(next proxy.RequestModifyFunc) proxy.RequestModifyFunc {
	return func(req *http.Request) {
		state := svc.projectState()
		projectID := state.id
		next(req)

		if projectID.Compare(ulid.ULID{}) == 0 {
			*req = *req.WithContext(context.WithValue(req.Context(), LogBypassedKey, true))
			return
		}

		clone := req.Clone(req.Context())

		var body []byte

		if req.Body != nil {
			var err error

			body, req.Body, err = captureBody(req.Body, svc.maxBodyBytes)
			if err != nil {
				svc.logger.Errorw("Failed to read request body for logging.",
					"error", err)
				*req = *req.WithContext(context.WithValue(req.Context(), LogBypassedKey, true))
				return
			}

			if int64(len(body)) == svc.maxBodyBytes {
				svc.logger.Infow("Request body truncated at max size.",
					"maxBodyBytes", svc.maxBodyBytes,
					"url", req.URL.String())
			}
		}

		// Bypass logging if this setting is enabled and the incoming request
		// doesn't match any scope rules.
		if state.bypassOutOfScope && !state.scope.Match(clone, body) {
			ctx := context.WithValue(req.Context(), LogBypassedKey, true)
			*req = *req.WithContext(ctx)

			svc.logger.Debugw("Bypassed logging: request doesn't match any scope rules.",
				"url", req.URL.String())

			return
		}

		reqID, ok := proxy.RequestIDFromContext(req.Context())
		if !ok {
			svc.logger.Errorw("Bypassed logging: request doesn't have an ID.")
			*req = *req.WithContext(context.WithValue(req.Context(), LogBypassedKey, true))
			return
		}

		// Redact sensitive headers on a copy so only the stored log is affected,
		// never the request still being forwarded upstream.
		header := clone.Header
		if len(svc.redactHeaders) > 0 {
			header = clone.Header.Clone()
			redactHeaders(header, svc.redactHeaders)
		}

		reqLog := RequestLog{
			ID:        reqID,
			ProjectID: projectID,
			Method:    clone.Method,
			URL:       clone.URL,
			Proto:     clone.Proto,
			Header:    header,
			Body:      body,
		}

		err := svc.repo.StoreRequestLog(req.Context(), reqLog)
		if err != nil {
			svc.logger.Errorw("Failed to store request log.",
				"error", err)
			ctx := context.WithValue(req.Context(), LogBypassedKey, true)
			*req = *req.WithContext(ctx)
			return
		}

		svc.logger.Debugw("Stored request log.",
			"reqLogID", reqLog.ID.String(),
			"url", reqLog.URL.String())

		svc.emit(sse.EventRequestLog)

		ctx := context.WithValue(req.Context(), projectIDKey, projectID)
		ctx = context.WithValue(ctx, ReqLogIDKey, reqLog.ID)
		*req = *req.WithContext(ctx)
	}
}

func (svc *Service) ResponseModifier(next proxy.ResponseModifyFunc) proxy.ResponseModifyFunc {
	return func(res *http.Response) error {
		if err := next(res); err != nil {
			return err
		}

		if bypassed, _ := res.Request.Context().Value(LogBypassedKey).(bool); bypassed {
			return nil
		}

		reqLogID, ok := res.Request.Context().Value(ReqLogIDKey).(ulid.ULID)
		if !ok {
			return errors.New("reqlog: request is missing ID")
		}

		duration, _ := proxy.RequestDurationFromContext(res.Request.Context())

		// Bind the response to the project that captured its request, even if
		// the user switches projects while the upstream request is in flight.
		projectID, ok := res.Request.Context().Value(projectIDKey).(ulid.ULID)
		if !ok {
			projectID = svc.ActiveProjectID()
		}
		clone := *res
		clone.Header = res.Header.Clone()

		if res.Body != nil {
			body, restored, err := captureBody(res.Body, svc.maxBodyBytes)
			res.Body = restored
			if err != nil {
				return fmt.Errorf("reqlog: could not read response body: %w", err)
			}

			if int64(len(body)) == svc.maxBodyBytes {
				svc.logger.Infow("Response body truncated at max size.",
					"maxBodyBytes", svc.maxBodyBytes)
			}

			clone.Body = io.NopCloser(bytes.NewBuffer(body))
		}

		resLog, err := ParseHTTPResponse(&clone)
		if err != nil {
			return err
		}
		resLog.Duration = duration
		redactHeaders(resLog.Header, svc.redactHeaders)

		go func() {
			if err := svc.repo.StoreResponseLog(context.Background(), projectID, reqLogID, resLog); err != nil {
				svc.logger.Errorw("Failed to store response log.", "error", err)
			} else {
				svc.logger.Debugw("Stored response log.", "reqLogID", reqLogID.String())
				svc.emit(sse.EventResponseLog)
			}
		}()

		return nil
	}
}

func (svc *Service) SetActiveProjectID(id ulid.ULID) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	svc.activeProjectID = id
	svc.findReqsFilter.ProjectID = id
}

func (svc *Service) ActiveProjectID() ulid.ULID {
	return svc.projectState().id
}

func (svc *Service) SetFindReqsFilter(filter FindRequestsFilter) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	filter.ProjectID = svc.activeProjectID
	svc.findReqsFilter = filter
}

func (svc *Service) FindReqsFilter() FindRequestsFilter {
	return svc.projectState().filter
}

func (svc *Service) SetBypassOutOfScopeRequests(bypass bool) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	svc.bypassOutOfScopeRequests = bypass
}

func (svc *Service) BypassOutOfScopeRequests() bool {
	return svc.projectState().bypassOutOfScope
}

func ParseHTTPResponse(res *http.Response) (ResponseLog, error) {
	var bodyReader io.Reader = http.NoBody
	if res.Body != nil {
		bodyReader = res.Body
	}

	if res.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(res.Body)
		if err != nil {
			return ResponseLog{}, fmt.Errorf("reqlog: could not create gzip reader: %w", err)
		}
		defer gzipReader.Close()
		bodyReader = gzipReader
	}

	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return ResponseLog{}, fmt.Errorf("reqlog: could not read body: %w", err)
	}

	header := res.Header.Clone()
	header.Del("Content-Encoding")

	return ResponseLog{
		Proto:      res.Proto,
		StatusCode: res.StatusCode,
		Status:     res.Status,
		Header:     header,
		Body:       body,
		Size:       int64(len(body)),
	}, nil
}
