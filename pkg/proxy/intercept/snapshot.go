package intercept

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
)

// Snapshots are private and immutable once published in a pending queue. Each
// caller receives cloned metadata and a new reader over the captured bytes.
// Contexts, parsed filter ASTs and TLS/certificate metadata remain read-only.
type requestSnapshot struct {
	metadata *http.Request
	body     []byte
	hasBody  bool
}

type responseSnapshot struct {
	metadata *http.Response
	body     []byte
	hasBody  bool
	request  *requestSnapshot
}

func bodyReader(body []byte) io.ReadCloser {
	if len(body) == 0 {
		return http.NoBody
	}
	return io.NopCloser(bytes.NewReader(body))
}

// readOwnedBody consumes and closes a stream owned exclusively by this caller.
// Body-size policy is handled separately from ownership (release task #32).
func readOwnedBody(body io.ReadCloser) ([]byte, error) {
	data, readErr := io.ReadAll(body)
	return data, errors.Join(readErr, body.Close())
}

// requestBody uses a replay reader when available; otherwise it captures the
// exclusive live body once and replaces it with a replayable in-memory body.
// This is not safe for concurrent access to the same caller-owned HTTP object.
func requestBody(req *http.Request) ([]byte, error) {
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		return readOwnedBody(body)
	}
	if req.Body == nil {
		return nil, nil
	}
	body, err := readOwnedBody(req.Body)
	req.Body = bodyReader(body)
	if err == nil {
		req.GetBody = func() (io.ReadCloser, error) { return bodyReader(body), nil }
	}
	return body, err
}

func captureRequest(req *http.Request) (*requestSnapshot, error) {
	// A caller may have edited Body without updating an old GetBody closure.
	// For a live request the current Body is authoritative.
	if req.Body != nil {
		req.GetBody = nil
	}
	body, err := requestBody(req)
	if err != nil {
		return nil, fmt.Errorf("intercept: failed to capture request body: %w", err)
	}
	metadata := req.Clone(req.Context())
	hasBody := req.Body != nil || req.GetBody != nil
	metadata.Body, metadata.GetBody = nil, nil
	metadata.Response = nil
	return &requestSnapshot{metadata: metadata, body: body, hasBody: hasBody}, nil
}

// A response can arrive after Transport consumed and closed Request.Body. Only
// GetBody can safely recover those bytes; never touch the live stream here.
// If interception was enabled after the request was sent, its body may be
// unavailable. Preserve metadata without pretending the consumed body is owned.
func captureSentRequest(req *http.Request) (*requestSnapshot, error) {
	if req == nil {
		return nil, errors.New("intercept: response is missing its request")
	}
	owned := req.Clone(req.Context())
	owned.Body = nil
	return captureRequest(owned)
}

func (s *requestSnapshot) clone() *http.Request {
	req := s.metadata.Clone(s.metadata.Context())
	if s.hasBody {
		req.Body = bodyReader(s.body)
		req.GetBody = func() (io.ReadCloser, error) { return bodyReader(s.body), nil }
	}
	return req
}

func captureResponse(res *http.Response, request *requestSnapshot) (*responseSnapshot, error) {
	if request == nil {
		var err error
		request, err = captureSentRequest(res.Request)
		if err != nil {
			return nil, err
		}
	}
	var body []byte
	hasBody := res.Body != nil
	if hasBody {
		var err error
		body, err = readOwnedBody(res.Body)
		res.Body = bodyReader(body)
		if err != nil {
			return nil, fmt.Errorf("intercept: failed to capture response body: %w", err)
		}
	}
	metadata := *res
	metadata.Header = res.Header.Clone()
	metadata.Trailer = res.Trailer.Clone()
	metadata.TransferEncoding = slices.Clone(res.TransferEncoding)
	metadata.Body, metadata.Request = nil, nil
	return &responseSnapshot{metadata: &metadata, body: body, hasBody: hasBody, request: request}, nil
}

func (s *responseSnapshot) clone() *http.Response {
	res := *s.metadata
	res.Header = s.metadata.Header.Clone()
	res.Trailer = s.metadata.Trailer.Clone()
	res.TransferEncoding = slices.Clone(s.metadata.TransferEncoding)
	res.Request = s.request.clone()
	if s.hasBody {
		res.Body = bodyReader(s.body)
	}
	return &res
}
