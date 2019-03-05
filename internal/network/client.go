package network

import (
	"crypto/tls"
	"fmt"
	"net"
	"sync"
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
	// ReadTimeout is the timeout associated with each read from a remote connection.
	ReadTimeout time.Duration
	// WriteTimeout is the timeout associated with each write to a remote connection.
	WriteTimeout time.Duration
}

// NewTLSClient creates a TLSClient pool, connected to a specified remote address.
// This procedure will establish the initial connections, perform TLS handshakes, and validate the
// server identity.
func NewTLSClient(addr string, serverName string, cxHook metrics.ConnectionLifecycleHook, opts TLSClientOpts) (*TLSClient, error) {
	cache := tls.NewLRUClientSessionCache(opts.PoolOpts.Capacity)
	conf := &tls.Config{
		ServerName:         serverName,
		ClientSessionCache: cache,
	}

	// The dialer wraps a standard TLS dial with R/W timeouts.
	dialer := func() (net.Conn, error) {
		conn, err := net.DialTimeout("tcp", addr, opts.ConnectTimeout)
		if err != nil {
			return nil, fmt.Errorf("client: error establishing connection: err=%v", err)
		}

		tlsConn := tls.Client(conn, conf)
		if err := tlsConn.Handshake(); err != nil {
			return nil, fmt.Errorf("client: TLS handshake failed: err=%v", err)
		}

		return NewTCPConn(tlsConn, opts.ReadTimeout, opts.WriteTimeout), nil
	}

	pool := NewPersistentConnPool(dialer, cxHook, opts.PoolOpts)

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
		c.statsMutex.Lock()
		defer c.statsMutex.Unlock()

		if err != nil {
			c.stats.FailedConnections++
		} else {
			c.stats.SuccessfulConnections++
		}
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
