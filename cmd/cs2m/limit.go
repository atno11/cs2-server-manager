package main

import (
	"net"
	"sync"
)

// limitedListener bounds simultaneously accepted connections, including slow clients.
type limitedListener struct {
	net.Listener
	slots chan struct{}
}

func newLimitedListener(inner net.Listener, limit int) net.Listener {
	return &limitedListener{Listener: inner, slots: make(chan struct{}, limit)}
}
func (l *limitedListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.slots <- struct{}{}:
			return &limitedConn{Conn: c, release: func() { <-l.slots }}, nil
		default:
			_ = c.Close()
		}
	}
}

type limitedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *limitedConn) Close() error { err := c.Conn.Close(); c.once.Do(c.release); return err }
