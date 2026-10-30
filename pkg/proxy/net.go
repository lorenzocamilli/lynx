package proxy

import (
	"errors"
	"net"
	"sync"
)

var ErrAlreadyAccepted = errors.New("listener already accepted")

// OnceAcceptListener implements net.Listener.
//
// Accepts a connection once and returns an error on subsequent
// attempts.
type OnceAcceptListener struct {
	c    net.Conn
	addr net.Addr
}

func (l *OnceAcceptListener) Accept() (net.Conn, error) {
	if l.c == nil {
		return nil, ErrAlreadyAccepted
	}

	c := l.c
	l.addr = c.LocalAddr()
	l.c = nil

	return c, nil
}

func (l *OnceAcceptListener) Close() error {
	return nil
}

func (l *OnceAcceptListener) Addr() net.Addr {
	if l.c != nil {
		return l.c.LocalAddr()
	}
	return l.addr
}

// ConnNotify embeds net.Conn and adds a channel field for notifying
// that the connection was closed.
type ConnNotify struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

func (c *ConnNotify) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { close(c.closed) })
	return err
}
