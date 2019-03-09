package network

import (
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"

	"dotproxy/internal/metrics"
)

// Client defines the interface for a TCP network client.
type Client interface {
	// Conn retrieves a single persistent connection.
	Conn() (*PersistentConn, error)

	// Stats returns historical client stats.
	Stats() Stats
}

// Stats formalizes stats tracked per-client.
type Stats struct {
	// SuccessfulConnections is the number of connections that the client has successfully
	// provided.
	SuccessfulConnections int
	// FailedConnections is the number of times that the client has failed to provide a
	// connection.
	FailedConnections int
}

// TLSClient describes a TLS_secured TCP client that recycles connections in a pool.
type TLSClient struct {
	addr       string
	cxHook     metrics.ConnectionLifecycleHook
	pool       *PersistentConnPool
	stats      Stats
	statsMutex sync.RWMutex
}

// TLSClientOpts formalizes TLS client configuration options.
type TLSClientOpts struct {
	// PoolOpts are connection pool-specific options.
	PoolOpts PersistentConnPoolOpts
	// ConnectTimeout is the timeout associated with establishing a connection with the remote
	// server.
	ConnectTimeout time.Duration
	// HandshakeTimeout is the timeout associated with performing a TLS handshake with the
	// remote server, after a connection has been successfully established.
	HandshakeTimeout time.Duration
	// ReadTimeout is the timeout associated with each read from a remote connection.
	ReadTimeout time.Duration
	// WriteTimeout is the timeout associated with each write to a remote connection.
	WriteTimeout time.Duration
}

const (
	// tcpFastOpenConnect is the TCP socket option constant (defined in the kernel)
	// controlling whether outgoing connections should use TCP Fast Open to reduce the number of
	// round trips, and thus overall latency, when re-establishing a TCP connection to a server.
	// It is not yet present in the syscall standard library for platform-agnostic usage.
	// https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/include/uapi/linux/tcp.h?h=v4.20#n120
	tcpFastOpenConnect = 30
)

// NewTLSClient creates a TLSClient pool, connected to a specified remote address.
// This procedure will establish the initial connections, perform TLS handshakes, and validate the
// server identity.
func NewTLSClient(addr string, serverName string, cxHook metrics.ConnectionLifecycleHook, opts TLSClientOpts) (*TLSClient, error) {
	// Use a custom dialer that sets the TCP Fast Open socket option and a connection timeout.
	dialer := &net.Dialer{
		Timeout: opts.ConnectTimeout,
		Control: func(network string, addr string, rc syscall.RawConn) error {
			return rc.Control(func(fd uintptr) {
				syscall.SetsockoptInt(
					int(fd),
					syscall.IPPROTO_TCP,
					tcpFastOpenConnect,
					1,
				)
			})
		},
	}

	conf := &tls.Config{
		ServerName:         serverName,
		ClientSessionCache: tls.NewLRUClientSessionCache(opts.PoolOpts.Capacity),
	}

	// The TLS dialer wraps the custom TCP dialer with a TLS encryption layer and R/W timeouts.
	tlsDialer := func() (net.Conn, error) {
		conn, err := dialer.Dial("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("client: error establishing connection: err=%v", err)
		}

		// Implicitly set a TLS handshake timeout by enforcing a R/W deadline on the
		// underlying connection.
		if opts.HandshakeTimeout > 0 {
			conn.SetDeadline(time.Now().Add(opts.HandshakeTimeout))
		}

		tlsConn := tls.Client(conn, conf)
		if err := tlsConn.Handshake(); err != nil {
			go conn.Close()
			return nil, fmt.Errorf("client: TLS handshake failed: err=%v", err)
		}

		return NewTCPConn(tlsConn, opts.ReadTimeout, opts.WriteTimeout), nil
	}

	pool := NewPersistentConnPool(tlsDialer, cxHook, opts.PoolOpts)

	return &TLSClient{
		addr:  addr,
		pool:  pool,
		stats: Stats{},
	}, nil
}

// Conn retrieves a single persistent connection from the pool.
func (c *TLSClient) Conn() (*PersistentConn, error) {
	conn, err := c.pool.Conn()

	defer func() {
		go func() {
			c.statsMutex.Lock()
			defer c.statsMutex.Unlock()

			if err != nil {
				c.stats.FailedConnections++
			} else {
				c.stats.SuccessfulConnections++
			}
		}()
	}()

	return conn, err
}

// Stats returns current client stats.
func (c *TLSClient) Stats() Stats {
	c.statsMutex.RLock()
	defer c.statsMutex.RUnlock()

	return c.stats
}

// String returns a string representation of the client.
func (c *TLSClient) String() string {
	return fmt.Sprintf("TLSClient{addr: %s, connections: %d}", c.addr, c.pool.Size())
}
