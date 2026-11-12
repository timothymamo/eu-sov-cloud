package main

import (
	"net"
	"sync"
)

// defaultMaxConnections is the default of --max-connections: the open
// connections the server holds at most (ADR 0019).
const defaultMaxConnections = 256

// limitListener holds at most cap(slots) connections open at once. A
// connection accepted over the cap is closed at once, never queued, so a
// client that opens many connections cannot make the server hold them all.
type limitListener struct {
	net.Listener

	slots chan struct{}
}

// newLimitListener wraps ln so that it holds at most n connections; n is at
// least 1.
func newLimitListener(ln net.Listener, n int) *limitListener {
	return &limitListener{Listener: ln, slots: make(chan struct{}, n)}
}

// Accept returns the next connection that fits under the cap, closing every
// connection that does not.
func (l *limitListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}

		select {
		case l.slots <- struct{}{}:
			return &limitConn{Conn: conn, release: func() { <-l.slots }}, nil
		default:
			_ = conn.Close()
		}
	}
}

// limitConn frees its listener's slot when it is closed, once.
type limitConn struct {
	net.Conn

	once    sync.Once
	release func()
}

// Close closes the connection and frees its slot.
func (c *limitConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.release)

	return err
}
