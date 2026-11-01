package intercept

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"

	"github.com/oklog/ulid"

	"github.com/lorenzocamilli/lynx/pkg/filter"
	"github.com/lorenzocamilli/lynx/pkg/log"
	"github.com/lorenzocamilli/lynx/pkg/proxy"
	"github.com/lorenzocamilli/lynx/pkg/sse"
)

var (
	ErrRequestAborted   = errors.New("intercept: request was aborted")
	ErrRequestNotFound  = errors.New("intercept: request not found")
	ErrRequestDone      = errors.New("intercept: request is done")
	ErrResponseNotFound = errors.New("intercept: response not found")
)

type contextKey int

const interceptResponseKey contextKey = 0

// Request owns the immutable snapshot and control channels for a pending request.
// All queue access is protected by Service.mu; HTTP streams are never published.
type Request struct {
	snapshot *requestSnapshot
	ctx      context.Context
	ch       chan *http.Request
	abort    chan struct{}
}

// Response owns the immutable snapshot and control channels for a pending response.
type Response struct {
	snapshot *responseSnapshot
	ctx      context.Context
	ch       chan *http.Response
	abort    chan struct{}
}

// Item is a caller-owned view. Its headers, URLs and body readers are independent
// of other readers and the forwarding goroutine. Context/TLS metadata is read-only.
type Item struct {
	Request  *http.Request
	Response *http.Response
}

type Service struct {
	mu          sync.RWMutex
	settings    Settings
	version     uint64
	requests    map[ulid.ULID]*Request
	responses   map[ulid.ULID]*Response
	logger      log.Logger
	broadcaster *sse.Broadcaster
}

type Config struct {
	Logger           log.Logger
	Broadcaster      *sse.Broadcaster
	RequestsEnabled  bool
	ResponsesEnabled bool
	RequestFilter    filter.Expression
	ResponseFilter   filter.Expression
}

// RequestIDs implements sort.Interface.
type RequestIDs []ulid.ULID

func NewService(cfg Config) *Service {
	s := &Service{
		requests:    make(map[ulid.ULID]*Request),
		responses:   make(map[ulid.ULID]*Response),
		logger:      cfg.Logger,
		broadcaster: cfg.Broadcaster,
		settings: Settings{
			RequestsEnabled:  cfg.RequestsEnabled,
			ResponsesEnabled: cfg.ResponsesEnabled,
			RequestFilter:    cfg.RequestFilter,
			ResponseFilter:   cfg.ResponseFilter,
		},
	}
	if s.logger == nil {
		s.logger = log.NewNopLogger()
	}
	return s
}

//nolint:unparam // Keep the event type explicit at the emission boundary.
func (svc *Service) emit(t sse.EventType) {
	if svc.broadcaster != nil {
		svc.broadcaster.Broadcast(sse.Event{Type: t})
	}
}

func (svc *Service) settingsSnapshot() (Settings, uint64) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.settings, svc.version
}

// RequestModifier intercepts requests before invoking the next modifier.
func (svc *Service) RequestModifier(next proxy.RequestModifyFunc) proxy.RequestModifyFunc {
	return func(req *http.Request) {
		modified, err := svc.InterceptRequest(req.Context(), req)
		if err != nil {
			if errors.Is(err, ErrRequestAborted) || errors.Is(err, context.Canceled) {
				svc.logger.Debugw("Stopping request interception.", "error", err)
			} else {
				svc.logger.Errorw("Failed to intercept request.", "error", err)
			}
			// Fail closed: a capture/filter error must not send a partially
			// consumed request upstream. Preserve the existing context values.
			ctx, cancel := context.WithCancel(req.Context())
			cancel()
			*req = *req.WithContext(ctx)
			return
		}
		*req = *modified
		next(req)
	}
}

// InterceptRequest owns req exclusively until it returns. It captures the body
// before publishing any pending item; readers never access the live HTTP object.
func (svc *Service) InterceptRequest(ctx context.Context, req *http.Request) (*http.Request, error) {
	id, ok := proxy.RequestIDFromContext(ctx)
	if !ok {
		svc.logger.Errorw("Failed to intercept: context doesn't have an ID.")
		return req, nil
	}
	settings, version := svc.settingsSnapshot()
	explicitResponse, _ := ShouldInterceptResponseFromContext(ctx)
	if !settings.RequestsEnabled && !settings.ResponsesEnabled && !explicitResponse {
		return req, nil
	}
	// Also preserve a replayable request body for later response inspection,
	// even when only response interception is enabled.
	snapshot, err := captureRequest(req)
	if err != nil {
		return nil, err
	}
	pending := &Request{snapshot: snapshot, ctx: ctx, ch: make(chan *http.Request, 1), abort: make(chan struct{})}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !settings.RequestsEnabled {
			return req, nil
		}
		if settings.RequestFilter != nil {
			match, err := MatchRequestFilter(snapshot.clone(), settings.RequestFilter)
			if err != nil {
				return nil, fmt.Errorf("intercept: failed to match request filter (id: %v): %w", id, err)
			}
			if !match {
				return req, nil
			}
		}
		svc.mu.Lock()
		if version != svc.version {
			// Settings changed during body preparation/filtering. Reevaluate
			// rather than registering a stale item after a disable/clear.
			settings, version = svc.settings, svc.version
			svc.mu.Unlock()
			continue
		}
		svc.requests[id] = pending
		svc.mu.Unlock()
		break
	}
	svc.emit(sse.EventIntercepted)
	defer func() {
		svc.mu.Lock()
		if svc.requests[id] == pending {
			delete(svc.requests, id)
		}
		svc.mu.Unlock()
		svc.emit(sse.EventIntercepted)
	}()
	select {
	case modified := <-pending.ch:
		if modified == nil {
			return nil, ErrRequestAborted
		}
		return modified, nil
	case <-pending.abort:
		return nil, ErrRequestAborted
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ModifyRequest prepares an owned copy before handing it to the forwarding
// goroutine. The caller must not mutate modReq during this call; afterward the
// caller and the forwarded request have independent headers and body readers.
func (svc *Service) ModifyRequest(id ulid.ULID, modReq *http.Request, modifyResponse *bool) error {
	svc.mu.RLock()
	pending, ok := svc.requests[id]
	svc.mu.RUnlock()
	if !ok {
		return ErrRequestNotFound
	}
	var owned *http.Request
	if modReq != nil {
		snapshot, err := captureRequest(modReq)
		if err != nil {
			return err
		}
		owned = snapshot.clone().WithContext(pending.snapshot.metadata.Context())
		if modifyResponse != nil {
			owned = owned.WithContext(WithInterceptResponse(owned.Context(), *modifyResponse))
		}
	}
	// Resolution and clearing share mu. The single buffered handoff cannot
	// block, and removing the entry ensures only one mutation can win.
	svc.mu.Lock()
	if svc.requests[id] != pending || pending.ctx.Err() != nil {
		svc.mu.Unlock()
		return ErrRequestDone
	}
	pending.ch <- owned
	delete(svc.requests, id)
	svc.mu.Unlock()
	svc.emit(sse.EventIntercepted)
	return nil
}

func (svc *Service) CancelRequest(id ulid.ULID) error {
	return svc.ModifyRequest(id, nil, nil)
}

// clearRequests requires mu. Closing an abort channel never waits for a
// forwarding goroutine to receive, so clearing cannot deadlock queue readers.
func (svc *Service) clearRequests() {
	for id, pending := range svc.requests {
		close(pending.abort)
		delete(svc.requests, id)
	}
}

func (svc *Service) clearResponses() {
	for id, pending := range svc.responses {
		close(pending.abort)
		delete(svc.responses, id)
	}
}

func (svc *Service) ClearRequests() {
	svc.mu.Lock()
	svc.clearRequests()
	svc.mu.Unlock()
	svc.emit(sse.EventIntercepted)
}

func (svc *Service) ClearResponses() {
	svc.mu.Lock()
	svc.clearResponses()
	svc.mu.Unlock()
	svc.emit(sse.EventIntercepted)
}

// Items returns independent, caller-owned pending item views in ID order.
func (svc *Service) Items() []Item {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	ids := make(RequestIDs, 0, len(svc.requests)+len(svc.responses))
	for id := range svc.requests {
		ids = append(ids, id)
	}
	for id := range svc.responses {
		if _, exists := svc.requests[id]; !exists {
			ids = append(ids, id)
		}
	}
	sort.Sort(ids)
	items := make([]Item, 0, len(ids))
	for _, id := range ids {
		items = append(items, svc.item(id))
	}
	return items
}

// UpdateSettings atomically changes admission policy and aborts queues on an
// enabled -> disabled transition. Filter ASTs are immutable after configuration.
func (svc *Service) UpdateSettings(settings Settings) {
	svc.mu.Lock()
	if svc.settings.RequestsEnabled && !settings.RequestsEnabled {
		svc.clearRequests()
	}
	if svc.settings.ResponsesEnabled && !settings.ResponsesEnabled {
		svc.clearResponses()
	}
	svc.settings = settings
	svc.version++
	svc.mu.Unlock()
	svc.emit(sse.EventIntercepted)
}

// item requires at least a read lock on mu.
func (svc *Service) item(id ulid.ULID) Item {
	var item Item
	if pending := svc.requests[id]; pending != nil {
		item.Request = pending.snapshot.clone()
	}
	if pending := svc.responses[id]; pending != nil {
		item.Response = pending.snapshot.clone()
	}
	return item
}

func (svc *Service) ItemByID(id ulid.ULID) (Item, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	if svc.requests[id] == nil && svc.responses[id] == nil {
		return Item{}, ErrRequestNotFound
	}
	return svc.item(id), nil
}

func (ids RequestIDs) Len() int           { return len(ids) }
func (ids RequestIDs) Less(i, j int) bool { return ids[i].Compare(ids[j]) == -1 }
func (ids RequestIDs) Swap(i, j int)      { ids[i], ids[j] = ids[j], ids[i] }

func WithInterceptResponse(ctx context.Context, value bool) context.Context {
	return context.WithValue(ctx, interceptResponseKey, value)
}

func ShouldInterceptResponseFromContext(ctx context.Context) (bool, bool) {
	value, ok := ctx.Value(interceptResponseKey).(bool)
	return value, ok
}

func (svc *Service) ResponseModifier(next proxy.ResponseModifyFunc) proxy.ResponseModifyFunc {
	return func(res *http.Response) error {
		modified, err := svc.InterceptResponse(res.Request.Context(), res)
		if err != nil {
			return fmt.Errorf("failed to intercept response: %w", err)
		}
		*res = *modified
		return next(res)
	}
}

// InterceptResponse exclusively owns res while preparing its private snapshot.
// The related request is reconstructed from GetBody, not its consumed live body.
func (svc *Service) InterceptResponse(ctx context.Context, res *http.Response) (*http.Response, error) {
	// Protocol upgrades are live bidirectional streams. Bypass even an
	// explicit response override: snapshotting would consume/close the
	// connection and lose Write, breaking ReverseProxy's upgrade handoff.
	if res.StatusCode == http.StatusSwitchingProtocols {
		return res, nil
	}
	id, ok := proxy.RequestIDFromContext(ctx)
	if !ok {
		svc.logger.Errorw("Failed to intercept: context doesn't have an ID.")
		return res, nil
	}
	explicit, specified := ShouldInterceptResponseFromContext(ctx)
	if specified && !explicit {
		return res, nil
	}
	settings, version := svc.settingsSnapshot()
	if !settings.ResponsesEnabled && !explicit {
		return res, nil
	}
	snapshot, err := captureResponse(res, nil)
	if err != nil {
		return nil, err
	}
	pending := &Response{snapshot: snapshot, ctx: ctx, ch: make(chan *http.Response, 1), abort: make(chan struct{})}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !settings.ResponsesEnabled && !explicit {
			return res, nil
		}
		if settings.ResponseFilter != nil {
			match, err := MatchResponseFilter(snapshot.clone(), settings.ResponseFilter)
			if err != nil {
				return nil, fmt.Errorf("intercept: failed to match response filter (id: %v): %w", id, err)
			}
			if !match {
				return res, nil
			}
		}
		svc.mu.Lock()
		if version != svc.version {
			settings, version = svc.settings, svc.version
			svc.mu.Unlock()
			continue
		}
		svc.responses[id] = pending
		svc.mu.Unlock()
		break
	}
	svc.emit(sse.EventIntercepted)
	defer func() {
		svc.mu.Lock()
		if svc.responses[id] == pending {
			delete(svc.responses, id)
		}
		svc.mu.Unlock()
		svc.emit(sse.EventIntercepted)
	}()
	select {
	case modified := <-pending.ch:
		if modified == nil {
			return nil, ErrRequestAborted
		}
		return modified, nil
	case <-pending.abort:
		return nil, ErrRequestAborted
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ModifyResponse transfers an independent copy to the response writer. Caller
// access to modRes must be exclusive for this call, as with ModifyRequest.
func (svc *Service) ModifyResponse(id ulid.ULID, modRes *http.Response) error {
	svc.mu.RLock()
	pending, ok := svc.responses[id]
	svc.mu.RUnlock()
	if !ok {
		return ErrRequestNotFound
	}
	var owned *http.Response
	if modRes != nil {
		snapshot, err := captureResponse(modRes, pending.snapshot.request)
		if err != nil {
			return err
		}
		owned = snapshot.clone()
	}
	svc.mu.Lock()
	if svc.responses[id] != pending || pending.ctx.Err() != nil {
		svc.mu.Unlock()
		return ErrRequestDone
	}
	pending.ch <- owned
	delete(svc.responses, id)
	svc.mu.Unlock()
	svc.emit(sse.EventIntercepted)
	return nil
}

func (svc *Service) CancelResponse(id ulid.ULID) error {
	return svc.ModifyResponse(id, nil)
}
