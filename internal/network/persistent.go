package network

import (
	"net"
)

// PersistentConnPool is a pool of persistent, long-lived connections. Connections are returned to
// the pool instead of closed for later reuse.
type PersistentConnPool struct {
	dialer func() (net.Conn, error)
	conns  chan net.Conn
}

// PersistentConn is a net.Conn that lazily closes connections; it invokes a closer callback
// function instead of actually closing the underlying connection. It also augments the net.Conn API
// by providing a Destroy() method that forcefully closes the underlying connection.
type PersistentConn struct {
	closer    func() error
	destroyed bool

	net.Conn
}

// NewPersistentConnPool creates a connection pool with the specified capacity and dialer factory.
// The capacity represents the maximum number of connections that may be held open in the pool
// (though depending on client and server side behaviors, the actual number of connections open at
// any time may be less than or greater than this capacity). The dialer is a net.Conn factory that
// describes how a new connection is created.
func NewPersistentConnPool(capacity int, dialer func() (net.Conn, error)) (*PersistentConnPool, error) {
	conns := make(chan net.Conn, capacity)

	// The entire pool is initially populated with live connections.
	for i := 0; i < capacity; i++ {
		conn, err := dialer()
		if err != nil {
			return nil, err
		}

		conns <- conn
	}

	return &PersistentConnPool{dialer, conns}, nil
}

// Conn returns a single connection. It may be a cached connection that already exists in the pool,
// or it may be a newly created connection in the event that the pool is empty.
func (p *PersistentConnPool) Conn() (*PersistentConn, error) {
	select {
	case conn := <-p.conns:
		closer := func() error { return p.put(conn) }

		return NewPersistentConn(conn, closer), nil
	default:
		conn, err := p.dialer()
		if err != nil {
			return nil, err
		}

		closer := func() error { return p.put(conn) }

		return NewPersistentConn(conn, closer), nil
	}
}

// put attempts to return a connection back to the pool, e.g. when it would otherwise be closed.
// The connection will be reinserted into the pool if there is sufficient capacity; otherwise, the
// connection is simply closed.
func (p *PersistentConnPool) put(conn net.Conn) error {
	select {
	case p.conns <- conn:
		return nil
	default:
		return conn.Close()
	}
}

// NewPersistentConn wraps an existing net.Conn with the specified close callback.
func NewPersistentConn(conn net.Conn, closer func() error) *PersistentConn {
	return &PersistentConn{closer: closer, Conn: conn}
}

// Close will invoke the close callback if the connection has not been destroyed; otherwise, it is
// a noop.
func (c *PersistentConn) Close() error {
	if !c.destroyed {
		return c.closer()
	}

	return nil
}

// Destroy closes the underlying connection. It has the same behavior as Close() in a standard
// net.Conn implementation.
func (c *PersistentConn) Destroy() error {
	c.destroyed = true
	return c.Conn.Close()
}
