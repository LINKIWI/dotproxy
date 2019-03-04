//go:generate stringer -type=LoadBalancingPolicy

package network

import (
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	"dotproxy/internal/metrics"
)

// LoadBalancingPolicy formalizes the load balancing decision policy to apply when proxying requests
// through a sharded network client.
type LoadBalancingPolicy int

// Client defines the interface for a TCP network client.
type Client interface {
	// Conn retrieves a single persistent connection.
	Conn() (*PersistentConn, error)

	// Connections returns the number of historical connections made by this client.
	Connections() int
}

// TLSClient describes a TLS_secured TCP client that recycles connections in a pool.
type TLSClient struct {
	addr        string
	cxHook      metrics.ConnectionLifecycleHook
	pool        *PersistentConnPool
	connections int
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

// ShardedClient is an abstraction to manage several Clients whose connections are supplied in
// accordance with a load balancing policy.
type ShardedClient struct {
	clients  []Client
	lbPolicy LoadBalancingPolicy
	rrIdx    int
}

const (
	// RoundRobin statefully iterates through each client on each connection request.
	RoundRobin LoadBalancingPolicy = iota
	// Random selects a client at random to provide the connection.
	Random
	// FewestHistoricalConnections selects the client that has, up until the time of request,
	// provided the fewest number of connections.
	FewestHistoricalConnections
)

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

		return NewTCPConn(tls.Client(conn, conf), opts.ReadTimeout, opts.WriteTimeout), nil
	}

	pool, err := NewPersistentConnPool(dialer, cxHook, opts.PoolOpts)
	if err != nil {
		return nil, fmt.Errorf("client: error creating connection pool: err=%v", err)
	}

	return &TLSClient{
		addr: addr,
		pool: pool,
	}, nil
}

// Conn retrieves a single persistent connection from the pool.
func (c *TLSClient) Conn() (*PersistentConn, error) {
	defer func() {
		c.connections++
	}()

	return c.pool.Conn()
}

// Connections reads the number of connections that this client has thus far provided.
func (c *TLSClient) Connections() int {
	return c.connections
}

// String returns a string representation of the client.
func (c *TLSClient) String() string {
	return fmt.Sprintf("TLSClient{addr: %s, connections: %d}", c.addr, c.pool.Size())
}

// NewShardedClient creates a single Client that provides connections from several other Clients
// governed by a load balancing policy.
func NewShardedClient(clients []Client, lbPolicy LoadBalancingPolicy) *ShardedClient {
	return &ShardedClient{clients: clients, lbPolicy: lbPolicy}
}

// Conn retrieves a single persistent connection from one of the clients.
func (c *ShardedClient) Conn() (*PersistentConn, error) {
	return c.selectClient().Conn()
}

// Connections returns the total number of connections provided by all fanout clients.
func (c *ShardedClient) Connections() int {
	var connections int

	for _, client := range c.clients {
		connections += client.Connections()
	}

	return connections
}

// String returns a string representation of the sharded client.
func (c *ShardedClient) String() string {
	return fmt.Sprintf("ShardedClient{lbPolicy: %s, clients: %v}", c.lbPolicy, c.clients)
}

// selectClient chooses a client to provide a connection.
func (c *ShardedClient) selectClient() Client {
	switch c.lbPolicy {
	case RoundRobin:
		return c.selectRoundRobin()
	case Random:
		return c.selectRandom()
	case FewestHistoricalConnections:
		return c.selectFewestHistoricalConnections()
	default:
		return c.selectRoundRobin()
	}
}

// selectRoundRobin selects the next client in line, while updating internal state to keep track of
// which client should be selected next.
func (c *ShardedClient) selectRoundRobin() Client {
	defer func() {
		c.rrIdx = (c.rrIdx + 1) % len(c.clients)
	}()

	return c.clients[c.rrIdx]
}

// selectRandom chooses a client at random.
func (c *ShardedClient) selectRandom() Client {
	return c.clients[rand.Intn(len(c.clients))]
}

// selectFewestHistoricalConnections selects the client that has provided the fewest connections
// until the snapshot in time at which this method is invoked.
func (c *ShardedClient) selectFewestHistoricalConnections() Client {
	var client Client

	for _, candidate := range c.clients {
		if client == nil || candidate.Connections() < client.Connections() {
			client = candidate
		}
	}

	return client
}

// ParseLoadBalancingPolicy parses a LoadBalancingPolicy constant from its stringified
// representation in a case-insensitive manner.
func ParseLoadBalancingPolicy(lbPolicy string) (LoadBalancingPolicy, bool) {
	knownLbPolicies := []LoadBalancingPolicy{
		RoundRobin,
		Random,
		FewestHistoricalConnections,
	}

	for _, knownLbPolicy := range knownLbPolicies {
		if strings.ToLower(lbPolicy) == strings.ToLower(knownLbPolicy.String()) {
			return knownLbPolicy, true
		}
	}

	return RoundRobin, false
}
