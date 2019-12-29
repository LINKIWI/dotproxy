package network

import (
	"fmt"
	"net"
	"time"

	"lib.kevinlin.info/aperture/lib"

	"dotproxy/internal/data"
	"dotproxy/internal/metrics"
)

// PersistentConnPool is a pool of persistent, long-lived connections. Connections are returned to
// the pool instead of closed for later reuse.
type PersistentConnPool struct {
	dialer       func() (net.Conn, error)
	cxHook       metrics.ConnectionLifecycleHook
	staleTimeout time.Duration
	conns        *data.MRUQueue
}

// PersistentConnPoolOpts formalizes configuration options for a persistent connection pool.
type PersistentConnPoolOpts struct {
	// Capacity is the maximum number of cached connections that may be held open in the pool.
	// Depending on client and server behaviors, the actual number of connections open at any
	// time may be less than or greater than this capacity. For example, there may be more
	// connections to serve a high number of concurrent clients, and there may be fewer
	// connections if many of them have been destroyed due to timeout or error.
	Capacity int
	// StaleTimeout is the duration after which a cached connection should be considered stale,
	// and thus reconnected before use. This represents the time between connection I/O events.
	StaleTimeout time.Duration
}

// PersistentConn is a net.Conn that lazily closes connections; it invokes a closer callback
// function instead of actually closing the underlying connection. It also augments the net.Conn API
// by providing a Destroy() method that forcefully closes the underlying connection.
type PersistentConn struct {
	closer    func(destroyed bool) error
	destroyed bool

	net.Conn
}

// NewPersistentConnPool creates a connection pool with the specified dialer factory and
// configuration options.  The dialer is a net.Conn factory that describes how a new connection is
// created.
func NewPersistentConnPool(dialer func() (net.Conn, error), cxHook metrics.ConnectionLifecycleHook, opts PersistentConnPoolOpts) *PersistentConnPool {
	conns := data.NewMRUQueue(opts.Capacity)

	// The entire pool is initially populated asynchronously with live connections, if possible.
	go func() {
		for i := 0; i < opts.Capacity; i++ {
			dialTimer := lib.NewStopwatch()
			conn, err := dialer()

			// It is nonideal, but not necessarily an error, if the pool cannot be
			// initially populated to the desired capacity. The size of the pool is
			// inherently variable, and pool clients generally degrade gracefully when
			// the pool fails to provide a connection.
			if err != nil {
				cxHook.EmitConnectionError()
			} else {
				cxHook.EmitConnectionOpen(dialTimer.Elapsed(), conn.RemoteAddr())
				conns.Push(conn)
			}
		}
	}()

	return &PersistentConnPool{
		dialer:       dialer,
		cxHook:       cxHook,
		staleTimeout: opts.StaleTimeout,
		conns:        conns,
	}
}

// Conn returns a single connection. It may be a cached connection that already exists in the pool,
// or it may be a newly created connection in the event that the pool is empty.
func (p *PersistentConnPool) Conn() (*PersistentConn, error) {
	value, timestamp, ok := p.conns.Pop()

	// Factory for creating a closer callback that closes the connection if it is destroyed, but
	// otherwise returns it to the cached connections pool.
	closerFactory := func(conn net.Conn) func(destroyed bool) error {
		return func(destroyed bool) error {
			if destroyed {
				p.cxHook.EmitConnectionClose(conn.RemoteAddr())
				return conn.Close()
			}

			return p.put(conn)
		}
	}

	// A cached connection is available; attempt to use it
	if ok {
		conn := value.(net.Conn)

		// The connection is not stale; use it
		if p.staleTimeout <= 0 || time.Since(timestamp) < p.staleTimeout {
			return NewPersistentConn(conn, closerFactory(conn)), nil
		}

		// The connection is stale; close it and open a new connection.
		// We are not particularly interested in propagating errors that may occur from
		// closing the connection, since it is already stale anyways.
		p.cxHook.EmitConnectionClose(conn.RemoteAddr())
		go conn.Close()
	}

	// A cached connection is not available or stale; create a new one
	dialTimer := lib.NewStopwatch()
	conn, err := p.dialer()
	if err != nil {
		p.cxHook.EmitConnectionError()
		return nil, err
	}

	p.cxHook.EmitConnectionOpen(dialTimer.Elapsed(), conn.RemoteAddr())

	return NewPersistentConn(conn, closerFactory(conn)), nil
}

// Size reports the current size of the connection pool.
func (p *PersistentConnPool) Size() int {
	return p.conns.Size()
}

// put attempts to return a connection back to the pool, e.g. when it would otherwise be closed.
// The connection will be reinserted into the pool if there is sufficient capacity; otherwise, the
// connection is simply closed.
func (p *PersistentConnPool) put(conn net.Conn) error {
	if ok := p.conns.Push(conn); !ok {
		return conn.Close()
	}

	return nil
}

// NewPersistentConn wraps an existing net.Conn with the specified close callback.
func NewPersistentConn(conn net.Conn, closer func(destroyed bool) error) *PersistentConn {
	return &PersistentConn{closer: closer, Conn: conn}
}

// Close will invoke the close callback if the connection has not been destroyed; otherwise, it is
// a noop. The callback is invoked with a single parameter describing whether the connection has
// been marked as destroyed; the interpretation of a destroyed connection is abstracted out to the
// PersistentConn callback supplier.
func (c *PersistentConn) Close() error {
	return c.closer(c.destroyed)
}

// Destroy markes the connection as destroyed and invokes the close callback.
func (c *PersistentConn) Destroy() error {
	c.destroyed = true

	return c.Close()
}

// String implements the Stringer interface for human-consumable representation.
func (c *PersistentConn) String() string {
	return fmt.Sprintf("PersistentConn{%s->%s}", c.LocalAddr(), c.RemoteAddr())
}
