package proxy

import (
	"errors"
	"net"
	"testing"
	"time"
)

func TestConnNotifyAndOnceAcceptListener(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	conn := &ConnNotify{Conn: server, closed: make(chan struct{})}
	var _ net.Conn = conn
	listener := &OnceAcceptListener{c: conn}
	addr := listener.Addr()
	accepted, err := listener.Accept()
	if err != nil || accepted != conn {
		t.Fatalf("Accept = %v, %v", accepted, err)
	}
	if listener.Addr() != addr {
		t.Fatal("listener address changed after Accept")
	}
	if _, err := listener.Accept(); !errors.Is(err, ErrAlreadyAccepted) {
		t.Fatalf("second Accept = %v", err)
	}
	done := make(chan struct{})
	go func() {
		_ = accepted.Close()
		_ = accepted.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close blocked without a notification receiver")
	}
	select {
	case <-conn.closed:
	default:
		t.Fatal("Close did not notify the tunnel handler")
	}
}
