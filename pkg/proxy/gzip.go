package proxy

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
)

// gzipBody keeps ownership of the upstream body while exposing decoded bytes.
// gzip.Reader.Close alone does not close the underlying network stream.
type gzipBody struct {
	*gzip.Reader
	upstream io.ReadCloser
}

func (b *gzipBody) Close() error {
	defer b.Reader.Close()
	return b.upstream.Close()
}

func gunzipResponseBody(res *http.Response) error {
	// A 101 body is the upgraded bidirectional connection, not an encoded
	// HTTP document. ReverseProxy must retain the original stream.
	if res.StatusCode == http.StatusSwitchingProtocols || res.Header.Get("Content-Encoding") != "gzip" {
		return nil
	}

	reader, err := gzip.NewReader(res.Body)
	if err != nil {
		return fmt.Errorf("proxy: could not create gzip reader: %w", err)
	}

	// Decode on demand rather than buffering the entire expanded response.
	// The decoded length is unknown until EOF; let net/http frame the stream.
	res.Body = &gzipBody{Reader: reader, upstream: res.Body}
	res.Header.Del("Content-Encoding")
	res.Header.Del("Content-Length")
	res.ContentLength = -1

	return nil
}
