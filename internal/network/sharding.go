//go:generate go run golang.org/x/tools/cmd/stringer -type=LoadBalancingPolicy

package network

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// LoadBalancingPolicy formalizes the load balancing decision policy to apply when proxying requests
// through a sharded network client.
type LoadBalancingPolicy int

// ShardedClientFactory is a type alias for a unary constructor function that returns a single
// Client that abstracts operations among several child Clients.
type ShardedClientFactory func([]Client) Client

// RoundRobinShardedClient shards requests among clients fairly in round-robin order.
type RoundRobinShardedClient struct {
	clients []Client

	// Current round robin index (not necessarily async-safe)
	rrIdx int
}

// RandomShardedClient shards requests among clients randomly.
type RandomShardedClient struct {
	clients []Client
}

// HistoricalConnectionsShardedClient directs requests to the client that has, up until the time of
// invocation, served the fewest number of successful connections. It is best used when there is a
// need to ensure that load is distributed to all clients fairly even if one of them has failed.
type HistoricalConnectionsShardedClient struct {
	clients []Client
}

// AvailabilityShardedClient provides connections by dynamically adjusting its active client pool to
// prioritize those clients that are successful in providing new connections. It automatically fails
// over failed client connection requests to healthy clients in the pool, temporarily disabling the
// failed client for future requests with an exponential backoff policy.
type AvailabilityShardedClient struct {
	clients []Client

	// Tracks the timestamp at which each client last errored
	lastError map[Client]time.Time
	// Tracks the current duration of time to wait before a failed connection is once again
	// available for use.
	errorExpiry map[Client]time.Duration
	// Mutex used to protect R/W operations on the state maps.
	mutex sync.RWMutex
}

// FailoverShardedClient provides connections in priority order, serially failing over to the next
// client(s) in the list when the primary is not successful in providing a connection.
type FailoverShardedClient struct {
	clients []Client
}

const (
	// RoundRobin statefully iterates through each client on every connection request.
	RoundRobin LoadBalancingPolicy = iota
	// Random selects a client at random to provide the connection.
	Random
	// HistoricalConnections selects the client that has, up until the time of request,
	// provided the  number of connections.
	HistoricalConnections
	// Availability randomly selects a client to provide the connection, failing over to another
	// client in the event that it fails to do so. The failed client is temporarily pulled out
	// of the availability pool to prevent subsequent requests from being directed to the failed
	// client.
	Availability
	// Failover provides connections from multiple clients in serial order, only failing over to
	// secondary clients when the primary fails.
	Failover
)

// NewShardedClient creates a single Client that provides connections from several other Clients
// governed by a load balancing policy. It returns an error if the specified load balancing policy
// has no associated sharded client factory.
func NewShardedClient(clients []Client, lbPolicy LoadBalancingPolicy) (Client, error) {
	factories := map[LoadBalancingPolicy]ShardedClientFactory{
		RoundRobin:            NewRoundRobinShardedClient,
		Random:                NewRandomShardedClient,
		HistoricalConnections: NewHistoricalConnectionsShardedClient,
		Availability:          NewAvailabilityShardedClient,
		Failover:              NewFailoverShardedClient,
	}

	factory, ok := factories[lbPolicy]
	if !ok {
		return nil, fmt.Errorf(
			"sharding: no factory configured for load balancing policy: policy=%s",
			lbPolicy,
		)
	}

	return factory(clients), nil
}

// NewRoundRobinShardedClient is a client factory for the round robin load balancing policy.
func NewRoundRobinShardedClient(clients []Client) Client {
	return &RoundRobinShardedClient{clients: clients}
}

// Conn retrieves a connection from the next client in the round robin index.
func (c *RoundRobinShardedClient) Conn() (*PersistentConn, error) {
	defer func() {
		c.rrIdx = (c.rrIdx + 1) % len(c.clients)
	}()

	return c.clients[c.rrIdx].Conn()
}

// Stats aggregates stats from all child clients.
func (c *RoundRobinShardedClient) Stats() Stats {
	return aggregateClientsStats(c.clients)
}

// NewRandomShardedClient is a client factory for the random load balancing policy.
func NewRandomShardedClient(clients []Client) Client {
	return &RandomShardedClient{clients}
}

// Conn selects a client at random to provide the connection.
func (c *RandomShardedClient) Conn() (*PersistentConn, error) {
	return c.clients[rand.Intn(len(c.clients))].Conn()
}

// Stats aggregates stats from all child clients.
func (c *RandomShardedClient) Stats() Stats {
	return aggregateClientsStats(c.clients)
}

// NewHistoricalConnectionsShardedClient is a client factory for the historical connections load
// balancing policy.
func NewHistoricalConnectionsShardedClient(clients []Client) Client {
	return &HistoricalConnectionsShardedClient{clients}
}

// Conn selects the client that has, up until the time of invocation, provided the fewest successful
// connections.
func (c *HistoricalConnectionsShardedClient) Conn() (*PersistentConn, error) {
	var client Client

	for _, candidate := range c.clients {
		if client == nil || candidate.Stats().SuccessfulConnections < client.Stats().SuccessfulConnections {
			client = candidate
		}
	}

	return client.Conn()
}

// Stats aggregates stats from all child clients.
func (c *HistoricalConnectionsShardedClient) Stats() Stats {
	return aggregateClientsStats(c.clients)
}

// NewAvailabilityShardedClient is a client factory for the availability load balancing policy.
func NewAvailabilityShardedClient(clients []Client) Client {
	lastError := make(map[Client]time.Time)
	errorExpiry := make(map[Client]time.Duration)

	for _, client := range clients {
		lastError[client] = time.Time{}
		errorExpiry[client] = 0
	}

	return &AvailabilityShardedClient{
		clients:     clients,
		lastError:   lastError,
		errorExpiry: errorExpiry,
	}
}

// Conn attempts to robustly provide a connection from all available client using a failover retry
// mechanism. It is possible for this method to error if the load balancing policy determines that
// there are no live clients eligible for providing a connection.
func (c *AvailabilityShardedClient) Conn() (*PersistentConn, error) {
	// Describes the amount of time that must elapse before resetting a client's error expiry
	// timer. In other words, this is the minimum amount of time after which a client errors
	// that it is permitted to be retried for a live connection. Otherwise, the connection is
	// pulled out of the sharding pool for exponentially increasing durations of time.
	failedClientExpiry := 30 * time.Second

	client, err := c.selectAvailable()
	if err != nil {
		return nil, err
	}

	conn, err := client.Conn()
	if err != nil {
		c.mutex.Lock()

		if c.lastError[client].IsZero() || time.Since(c.lastError[client]) > failedClientExpiry {
			// The client has either never errored before, or the last error is too far
			// in the past. Start its exponential backoff timer at 100 ms, indicating
			// that this client will be marked unavailable for the next 100 ms.
			c.errorExpiry[client] = 100 * time.Millisecond
		} else {
			// The most recent client failure was too recent; double the current expiry
			// time.
			c.errorExpiry[client] *= 2
		}

		c.lastError[client] = time.Now()

		c.mutex.Unlock()

		return c.Conn()
	}

	return conn, nil
}

// Stats aggregates stats from all child clients.
func (c *AvailabilityShardedClient) Stats() Stats {
	return aggregateClientsStats(c.clients)
}

// Select an eligible client at random. This method may error if no clients are available to
// provide connections.
func (c *AvailabilityShardedClient) selectAvailable() (Client, error) {
	var eligibleClients []Client

	for _, candidate := range c.clients {
		c.mutex.RLock()
		lastError := c.lastError[candidate]
		expiry := c.errorExpiry[candidate]
		c.mutex.RUnlock()

		// The client is considered eligible if it has never errored or if its current
		// failure lifetime has expired.
		if lastError.IsZero() || time.Since(lastError) > expiry {
			eligibleClients = append(eligibleClients, candidate)
		}
	}

	if len(eligibleClients) == 0 {
		return nil, fmt.Errorf("sharding: no live clients are available")
	}

	return eligibleClients[rand.Intn(len(eligibleClients))], nil
}

// NewFailoverShardedClient is a client factory for the failover load balancing policy.
func NewFailoverShardedClient(clients []Client) Client {
	return &FailoverShardedClient{clients}
}

// Conn attempts to provide connections from clients in serial order, failing over to the next
// client on error.
func (c *FailoverShardedClient) Conn() (*PersistentConn, error) {
	for _, client := range c.clients {
		if conn, err := client.Conn(); err == nil {
			return conn, nil
		}
	}

	return nil, fmt.Errorf("sharding: all clients failed to provide a connection")
}

// Stats aggregates stats from all child clients.
func (c *FailoverShardedClient) Stats() Stats {
	return aggregateClientsStats(c.clients)
}

// ParseLoadBalancingPolicy parses a LoadBalancingPolicy constant from its stringified
// representation in a case-insensitive manner.
func ParseLoadBalancingPolicy(lbPolicy string) (LoadBalancingPolicy, bool) {
	knownLbPolicies := []LoadBalancingPolicy{
		RoundRobin,
		Random,
		HistoricalConnections,
		Availability,
		Failover,
	}

	for _, knownLbPolicy := range knownLbPolicies {
		if strings.ToLower(lbPolicy) == strings.ToLower(knownLbPolicy.String()) {
			return knownLbPolicy, true
		}
	}

	return RoundRobin, false
}

// aggregateClientsStats creates a single Stats struct from those in multiple Clients.
func aggregateClientsStats(clients []Client) Stats {
	var multipleStats []Stats
	var aggregatedStats Stats

	for _, client := range clients {
		multipleStats = append(multipleStats, client.Stats())
	}

	for _, stats := range multipleStats {
		aggregatedStats.SuccessfulConnections += stats.SuccessfulConnections
		aggregatedStats.FailedConnections += stats.FailedConnections
	}

	return aggregatedStats
}
