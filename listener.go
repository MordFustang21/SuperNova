package supernova

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

// GracefulListener is used as custom listener to watch connections
type GracefulListener struct {
	// inner listener
	ln net.Listener

	// maximum wait time for graceful shutdown
	maxWaitTime time.Duration

	// this channel is closed during graceful shutdown on zero open connections.
	done chan struct{}

	// the number of open connections
	connsCount uint64

	// becomes non-zero when graceful shutdown starts
	shutdown uint64
}

// NewGracefulListener wraps the given listener into 'graceful shutdown' listener.
func NewGracefulListener(ln net.Listener, maxWaitTime time.Duration) net.Listener {
	return &GracefulListener{
		ln:          ln,
		maxWaitTime: maxWaitTime,
		done:        make(chan struct{}),
	}
}

// Accept waits for connection increments count and returns to the listener.
func (ln *GracefulListener) Accept() (net.Conn, error) {
	c, err := ln.ln.Accept()
	if err != nil {
		return nil, err
	}
	atomic.AddUint64(&ln.connsCount, 1)
	return &gracefulConn{
		Conn: c,
		ln:   ln,
	}, nil
}

// Close closes the inner listener and waits until all the pending open connections
// are closed before returning.
func (ln *GracefulListener) Close() error {
	err := ln.ln.Close()
	if err != nil {
		return nil
	}
	return ln.waitForZeroConns()
}

// Addr returns the listener's network address.
func (ln *GracefulListener) Addr() net.Addr {
	return ln.ln.Addr()
}

func (ln *GracefulListener) waitForZeroConns() error {
	atomic.AddUint64(&ln.shutdown, 1)
	fmt.Printf("Waiting on %d connections\n", ln.connsCount)
	select {
	case <-ln.done:
		return nil
	case <-time.After(ln.maxWaitTime):
		return fmt.Errorf("cannot complete graceful shutdown in %s", ln.maxWaitTime)
	}
}

func (ln *GracefulListener) closeConn() {
	connsCount := atomic.AddUint64(&ln.connsCount, ^uint64(0))
	if atomic.LoadUint64(&ln.shutdown) != 0 && connsCount == 0 {
		close(ln.done)
	}
}

type gracefulConn struct {
	net.Conn
	ln *GracefulListener
}

// Close starts listener shutdown
func (c *gracefulConn) Close() error {
	err := c.Conn.Close()
	if err != nil {
		return err
	}
	c.ln.closeConn()
	return nil
}
