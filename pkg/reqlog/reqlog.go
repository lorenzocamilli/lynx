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
}

type Service struct {
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

func (svc *Service) FindRequests(ctx context.Context, limit, offset int) ([]RequestLog, error) {
	filter := svc.findReqsFilter
	filter.Limit = limit
	filter.Offset = offset
	return svc.repo.FindRequestLogs(ctx, filter, svc.scope)
}

func (svc *Service) FindRequestLogByID(ctx context.Context, id ulid.ULID) (RequestLog, error) {
	return svc.repo.FindRequestLogByID(ctx, svc.activeProjectID, id)
}

func (svc *Service) CountRequests(ctx context.Context) (int, error) {
	return svc.repo.CountRequestLogs(ctx, svc.activeProjectID)
}

func (svc *Service) ClearRequests(ctx context.Context, projectID ulid.ULID) error {
	return svc.repo.ClearRequestLogs(ctx, projectID)
}

func (svc *Service) DeleteRequest(ctx context.Context, projectID, id ulid.ULID) error {
	return svc.repo.DeleteRequestLog(ctx, projectID, id)
}

func (svc *Service) storeResponse(ctx context.Context, reqLogID ulid.ULID, res *http.Response) error {
	resLog, err := ParseHTTPResponse(res)
	if err != nil {
		return err
	}

	// resLog.Header is already a clone (see ParseHTTPResponse), so redacting
	// in place only affects the stored log, not the forwarded response.
	if len(svc.redactHeaders) > 0 {
		redactHeaders(resLog.Header, svc.redactHeaders)
	}

	return svc.repo.StoreResponseLog(ctx, svc.activeProjectID, reqLogID, resLog)
}

func (svc *Service) RequestModifier(next proxy.RequestModifyFunc) proxy.RequestModifyFunc {
	return func(req *http.Request) {
		next(req)

		clone := req.Clone(req.Context())

		var body []byte

		if req.Body != nil {
			var err error

			body, err = io.ReadAll(io.LimitReader(req.Body, svc.maxBodyBytes))
			if err != nil {
				svc.logger.Errorw("Failed to read request body for logging.",
					"error", err)
				return
			}

			if int64(len(body)) == svc.maxBodyBytes {
				svc.logger.Infow("Request body truncated at max size.",
					"maxBodyBytes", svc.maxBodyBytes,
					"url", req.URL.String())
			}

			req.Body = io.NopCloser(bytes.NewBuffer(body))
			clone.Body = io.NopCloser(bytes.NewBuffer(body))
		}

		// Bypass logging if no project is active.
		if svc.activeProjectID.Compare(ulid.ULID{}) == 0 {
			ctx := context.WithValue(req.Context(), LogBypassedKey, true)
			*req = *req.WithContext(ctx)

			svc.logger.Debugw("Bypassed logging: no active project.",
				"url", req.URL.String())

			return
		}

		// Bypass logging if this setting is enabled and the incoming request
		// doesn't match any scope rules.
		if svc.bypassOutOfScopeRequests && !svc.scope.Match(clone, body) {
			ctx := context.WithValue(req.Context(), LogBypassedKey, true)
			*req = *req.WithContext(ctx)

			svc.logger.Debugw("Bypassed logging: request doesn't match any scope rules.",
				"url", req.URL.String())

			return
		}

		reqID, ok := proxy.RequestIDFromContext(req.Context())
		if !ok {
			svc.logger.Errorw("Bypassed logging: request doesn't have an ID.")
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
			ProjectID: svc.activeProjectID,
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

		ctx := context.WithValue(req.Context(), ReqLogIDKey, reqLog.ID)
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

		clone := *res

		if res.Body != nil {
			body, err := io.ReadAll(io.LimitReader(res.Body, svc.maxBodyBytes))
			if err != nil {
				return fmt.Errorf("reqlog: could not read response body: %w", err)
			}

			if int64(len(body)) == svc.maxBodyBytes {
				svc.logger.Infow("Response body truncated at max size.",
					"maxBodyBytes", svc.maxBodyBytes)
			}

			res.Body = io.NopCloser(bytes.NewBuffer(body))
			clone.Body = io.NopCloser(bytes.NewBuffer(body))
		}

		go func() {
			if err := svc.storeResponse(context.Background(), reqLogID, &clone); err != nil {
				svc.logger.Errorw("Failed to store response log.",
					"error", err)
			} else {
				svc.logger.Debugw("Stored response log.",
					"reqLogID", reqLogID.String())
				svc.emit(sse.EventResponseLog)
			}
		}()

		return nil
	}
}

func (svc *Service) SetActiveProjectID(id ulid.ULID) {
	svc.activeProjectID = id
}

func (svc *Service) ActiveProjectID() ulid.ULID {
	return svc.activeProjectID
}

func (svc *Service) SetFindReqsFilter(filter FindRequestsFilter) {
	svc.findReqsFilter = filter
}

func (svc *Service) FindReqsFilter() FindRequestsFilter {
	return svc.findReqsFilter
}

func (svc *Service) SetBypassOutOfScopeRequests(bypass bool) {
	svc.bypassOutOfScopeRequests = bypass
}

func (svc *Service) BypassOutOfScopeRequests() bool {
	return svc.bypassOutOfScopeRequests
}

func ParseHTTPResponse(res *http.Response) (ResponseLog, error) {
	bodyReader := res.Body

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
	}, nil
}
