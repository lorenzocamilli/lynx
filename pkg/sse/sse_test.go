package sse_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lorenzocamilli/lynx/pkg/sse"
)

func TestBroadcaster_SingleClient(t *testing.T) {
	t.Parallel()

	b := sse.NewBroadcaster()
	srv := httptest.NewServer(sse.Handler(b))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error connecting to SSE server: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %q", ct)
	}

	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				lines <- line
				return
			}
		}
	}()

	// Give the handler time to subscribe before broadcasting.
	time.Sleep(20 * time.Millisecond)
	b.Broadcast(sse.Event{Type: sse.EventRequestLog})

	select {
	case line := <-lines:
		if !strings.Contains(line, `"type":"request_log"`) {
			t.Fatalf("unexpected SSE data line: %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for SSE event")
	}
}

func TestBroadcaster_MultipleClients(t *testing.T) {
	t.Parallel()

	b := sse.NewBroadcaster()
	srv := httptest.NewServer(sse.Handler(b))
	defer srv.Close()

	connect := func(t *testing.T) (*http.Response, chan string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t.Cleanup(cancel)

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to connect SSE client: %v", err)
		}

		ch := make(chan string, 1)
		go func() {
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data:") {
					ch <- line
					return
				}
			}
		}()
		return resp, ch
	}

	resp1, ch1 := connect(t)
	defer resp1.Body.Close()
	resp2, ch2 := connect(t)
	defer resp2.Body.Close()

	time.Sleep(20 * time.Millisecond)
	b.Broadcast(sse.Event{Type: sse.EventIntercepted})

	for i, ch := range []chan string{ch1, ch2} {
		select {
		case line := <-ch:
			if !strings.Contains(line, `"type":"intercepted"`) {
				t.Fatalf("client %d: unexpected SSE data: %q", i+1, line)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("client %d: timed out waiting for SSE event", i+1)
		}
	}
}

func TestBroadcaster_ClientDisconnect(t *testing.T) {
	t.Parallel()

	b := sse.NewBroadcaster()
	srv := httptest.NewServer(sse.Handler(b))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	// Cancel the client context to simulate disconnect.
	cancel()
	resp.Body.Close()

	// Broadcast after disconnect — should not block or panic.
	done := make(chan struct{})
	go func() {
		b.Broadcast(sse.Event{Type: sse.EventResponseLog})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Broadcast blocked after client disconnect")
	}
}
