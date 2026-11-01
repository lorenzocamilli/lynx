package proxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"
)

type closeTrackingBody struct {
	io.Reader
	closed bool
}

func (b *closeTrackingBody) Close() error {
	b.closed = true
	return nil
}

func TestGunzipResponseBodyStreamsAndClosesUpstream(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	payload := bytes.Repeat([]byte("a"), 1024*1024)
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	original := &closeTrackingBody{Reader: bytes.NewReader(compressed.Bytes())}
	res := &http.Response{
		Body:          original,
		Header:        http.Header{"Content-Encoding": {"gzip"}, "Content-Length": {"123"}},
		ContentLength: 123,
	}
	if err := gunzipResponseBody(res); err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Body.(*gzipBody); !ok {
		t.Fatal("expected a streaming gzip body")
	}
	if res.ContentLength != -1 || res.Header.Get("Content-Length") != "" || res.Header.Get("Content-Encoding") != "" {
		t.Fatal("encoded framing headers were not removed")
	}
	decoded, err := io.ReadAll(res.Body)
	if err != nil || !bytes.Equal(decoded, payload) {
		t.Fatalf("decoded body differs: %v", err)
	}
	if err := res.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if !original.closed {
		t.Fatal("upstream body was not closed")
	}
}

func TestGunzipDoesNotReadUpgradeStream(t *testing.T) {
	original := &closeTrackingBody{Reader: strings.NewReader("not gzip: a live protocol stream")}
	res := &http.Response{
		StatusCode:    http.StatusSwitchingProtocols,
		Body:          original,
		Header:        http.Header{"Content-Encoding": {"gzip"}, "Content-Length": {"123"}},
		ContentLength: 123,
	}
	if err := gunzipResponseBody(res); err != nil {
		t.Fatal(err)
	}
	if res.Body != original || original.closed || res.Header.Get("Content-Encoding") != "gzip" || res.ContentLength != 123 {
		t.Fatal("upgrade stream or framing metadata changed")
	}
	data, err := io.ReadAll(res.Body)
	if err != nil || string(data) != "not gzip: a live protocol stream" {
		t.Fatalf("upgrade bytes were consumed: %q, %v", data, err)
	}
}
