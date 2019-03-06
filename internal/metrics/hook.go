package metrics

import (
	"fmt"
	"net"
	"os"
	"time"
)

// ConnectionLifecycleHook is a metrics hook interface for reporting events that occur during a TCP
// connection lifecycle. Note that it is not pertinent to UDP transports, since it is an inherently
// connectionless protocol.
type ConnectionLifecycleHook interface {
	// EmitConnectionOpen reports the event that a connection was successfully opened.
	EmitConnectionOpen(latency time.Duration, addr net.Addr)

	// EmitConnectionClose reports the event that a connection was closed.
	EmitConnectionClose(addr net.Addr)

	// EmitConnectionError reports occurrence of an error establishing a connection.
	EmitConnectionError()
}

// ConnectionIOHook is a metrics hook interface for reporting events related to I/O with an
// established TCP or UDP connection.
type ConnectionIOHook interface {
	// EmitReadError reports the event that a connection read failed.
	EmitReadError(addr net.Addr)

	// EmitWriteError reports the event that a connection write failed.
	EmitWriteError(addr net.Addr)

	// EmitRetry reports the event that an I/O operation was retried due to failure.
	EmitRetry(addr net.Addr)
}

// ProxyHook is a metrics hook interface for reporting events and latencies related to end-to-end
// proxying of a client request with an upstream server.
type ProxyHook interface {
	// EmitRequestSize reports the size of the proxied request on the wire.
	EmitRequestSize(bytes int64, client net.Addr)

	// EmitResponseSize reports the size of the proxied response on the wire.
	EmitResponseSize(bytes int64, upstream net.Addr)

	// EmitRTT reports the total, end-to-end latency associated with serving a single request
	// from a client. This includes the time to establish/teardown all connections, transact
	// with the upstream, and proxy the response to/from the client.
	EmitRTT(latency time.Duration, client net.Addr, upstream net.Addr)

	// EmitUpstreamLatency reports the latency associated with transacting with the upstream
	// to serve a single request.
	EmitUpstreamLatency(latency time.Duration, client net.Addr, upstream net.Addr)

	// EmitError reports the occurrence of a critical error in the proxy lifecycle that causes
	// the request to not be correctly served.
	EmitError()
}

// AsyncStatsdConnectionLifecycleHook is an implementation of ConnectionLifecycleHook that outputs
// metrics asynchronously to statsd.
type AsyncStatsdConnectionLifecycleHook struct {
	client *StatsdClient
	source string
}

// AsyncStatsdConnectionIOHook is an implementation of ConnectionIOHook that outputs metrics
// asynchronously to statsd.
type AsyncStatsdConnectionIOHook struct {
	client *StatsdClient
	source string
}

// AsyncStatsdProxyHook is an implementation of ProxyHook that outputs metrics asynchronously to
// statsd.
type AsyncStatsdProxyHook struct {
	client *StatsdClient
}

// NoopConnectionLifecycleHook implements the ConnectionLifecycleHook interface but noops on all
// emissions.
type NoopConnectionLifecycleHook struct{}

// NoopConnectionIOHook implements the ConnectionIOHook interface but noops on all emissions.
type NoopConnectionIOHook struct{}

// NoopProxyHook implements the ProxyHook interface but noops on all emissions.
type NoopProxyHook struct{}

// NewAsyncStatsdConnectionLifecycleHook creates a new client with the specified source, statsd
// address, and statsd sample rate. The source denotes the entity with whom the server is opening
// and closing TCP connections.
func NewAsyncStatsdConnectionLifecycleHook(source string, addr string, sampleRate float32) (ConnectionLifecycleHook, error) {
	client, err := statsdClientFactory(addr, sampleRate)
	if err != nil {
		return nil, err
	}

	return &AsyncStatsdConnectionLifecycleHook{
		client: client,
		source: source,
	}, nil
}

// EmitConnectionOpen statsd implementation
func (h *AsyncStatsdConnectionLifecycleHook) EmitConnectionOpen(latency time.Duration, addr net.Addr) {
	go func() {
		tags := map[string]string{
			"addr":      ipFromAddr(addr),
			"transport": transportFromAddr(addr),
		}

		h.client.Count(fmt.Sprintf("event.%s.cx_open", h.source), 1, tags)

		if latency > 0 {
			h.client.Timing(fmt.Sprintf("latency.%s.cx_open", h.source), latency, tags)
		}
	}()
}

// EmitConnectionClose statsd implementation
func (h *AsyncStatsdConnectionLifecycleHook) EmitConnectionClose(addr net.Addr) {
	go h.client.Count(fmt.Sprintf("event.%s.cx_close", h.source), 1, map[string]string{
		"addr":      ipFromAddr(addr),
		"transport": transportFromAddr(addr),
	})
}

// EmitConnectionError statsd implementation
func (h *AsyncStatsdConnectionLifecycleHook) EmitConnectionError() {
	go h.client.Count(fmt.Sprintf("event.%s.cx_error", h.source), 1, nil)
}

// NewNoopConnectionLifecycleHook creates a noop implementation of ConnectionLifecycleHook.
func NewNoopConnectionLifecycleHook() ConnectionLifecycleHook {
	return &NoopConnectionLifecycleHook{}
}

// EmitConnectionOpen noops.
func (h *NoopConnectionLifecycleHook) EmitConnectionOpen(latency time.Duration, addr net.Addr) {}

// EmitConnectionClose noops.
func (h *NoopConnectionLifecycleHook) EmitConnectionClose(addr net.Addr) {}

// EmitConnectionError noops.
func (h *NoopConnectionLifecycleHook) EmitConnectionError() {}

// NewAsyncStatsdConnectionIOHook creates a new client with the specified source, statsd address,
// and statsd sample rate. The source denotes the entity with whom the server is performing I/O.
func NewAsyncStatsdConnectionIOHook(source string, addr string, sampleRate float32) (ConnectionIOHook, error) {
	client, err := statsdClientFactory(addr, sampleRate)
	if err != nil {
		return nil, err
	}

	return &AsyncStatsdConnectionIOHook{
		client: client,
		source: source,
	}, nil
}

// EmitReadError statsd implementation.
func (h *AsyncStatsdConnectionIOHook) EmitReadError(addr net.Addr) {
	go h.client.Count(fmt.Sprintf("event.%s.read_error", h.source), 1, map[string]string{
		"addr":      ipFromAddr(addr),
		"transport": transportFromAddr(addr),
	})
}

// EmitWriteError statsd implementation.
func (h *AsyncStatsdConnectionIOHook) EmitWriteError(addr net.Addr) {
	go h.client.Count(fmt.Sprintf("event.%s.write_error", h.source), 1, map[string]string{
		"addr":      ipFromAddr(addr),
		"transport": transportFromAddr(addr),
	})
}

// EmitRetry statsd implementation.
func (h *AsyncStatsdConnectionIOHook) EmitRetry(addr net.Addr) {
	go h.client.Count(fmt.Sprintf("event.%s.io_retry", h.source), 1, map[string]string{
		"addr":      ipFromAddr(addr),
		"transport": transportFromAddr(addr),
	})
}

// NewNoopConnectionIOHook creates a noop implementation of ConnectionIOHook.
func NewNoopConnectionIOHook() ConnectionIOHook {
	return &NoopConnectionIOHook{}
}

// EmitReadError noops.
func (h *NoopConnectionIOHook) EmitReadError(addr net.Addr) {}

// EmitWriteError noops.
func (h *NoopConnectionIOHook) EmitWriteError(addr net.Addr) {}

// EmitRetry noops.
func (h *NoopConnectionIOHook) EmitRetry(addr net.Addr) {}

// NewAsyncStatsdProxyHook creates a new client with the specified statsd address and sample rate.
func NewAsyncStatsdProxyHook(addr string, sampleRate float32) (ProxyHook, error) {
	client, err := statsdClientFactory(addr, sampleRate)
	if err != nil {
		return nil, err
	}

	return &AsyncStatsdProxyHook{client}, nil
}

// EmitRequestSize statsd implementation
func (h *AsyncStatsdProxyHook) EmitRequestSize(bytes int64, client net.Addr) {
	go h.client.Size("size.proxy.request", bytes, map[string]string{
		"addr": ipFromAddr(client),
	})
}

// EmitResponseSize statsd implementation
func (h *AsyncStatsdProxyHook) EmitResponseSize(bytes int64, upstream net.Addr) {
	go h.client.Size("size.proxy.response", bytes, map[string]string{
		"addr": ipFromAddr(upstream),
	})
}

// EmitRTT statsd implementation
func (h *AsyncStatsdProxyHook) EmitRTT(latency time.Duration, client net.Addr, upstream net.Addr) {
	go h.client.Timing("latency.proxy.tx_rtt", latency, map[string]string{
		"client":    ipFromAddr(client),
		"upstream":  ipFromAddr(upstream),
		"transport": transportFromAddr(client),
	})
}

// EmitUpstreamLatency statsd implementation
func (h *AsyncStatsdProxyHook) EmitUpstreamLatency(latency time.Duration, client net.Addr, upstream net.Addr) {
	go h.client.Timing("latency.proxy.tx_upstream", latency, map[string]string{
		"client":   ipFromAddr(client),
		"upstream": ipFromAddr(upstream),
	})
}

// EmitError statsd implementation
func (h *AsyncStatsdProxyHook) EmitError() {
	go h.client.Count("event.proxy.error", 1, nil)
}

// NewNoopProxyHook creates a noop implementation of ProxyHook.
func NewNoopProxyHook() ProxyHook {
	return &NoopProxyHook{}
}

// EmitRequestSize noops.
func (h *NoopProxyHook) EmitRequestSize(bytes int64, client net.Addr) {}

// EmitResponseSize noops.
func (h *NoopProxyHook) EmitResponseSize(bytes int64, upstream net.Addr) {}

// EmitRTT noops.
func (h *NoopProxyHook) EmitRTT(latency time.Duration, client net.Addr, upstream net.Addr) {}

// EmitUpstreamLatency noops.
func (h *NoopProxyHook) EmitUpstreamLatency(latency time.Duration, client net.Addr, upstream net.Addr) {
}

// EmitError noops.
func (h *NoopProxyHook) EmitError() {}

// statsdClientFactory creates a configured StatsdClient with reasonable defaults for the given
// statsd server address and sample rate.
func statsdClientFactory(addr string, sampleRate float32) (*StatsdClient, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	defaultTags := map[string]string{
		"host": hostname,
	}

	return NewStatsdClient(addr, "dotproxy", defaultTags, sampleRate)
}

// ipFromAddr returns the IP address from a full net.Addr, or null if unavailable.
func ipFromAddr(addr net.Addr) string {
	switch networkAddr := addr.(type) {
	case *net.UDPAddr:
		return networkAddr.IP.String()
	case *net.TCPAddr:
		return networkAddr.IP.String()
	default:
		return "null"
	}
}

// transportFromAddr returns the transport protocol (as a string) behind a net.Addr, or null if
// unavailable.
func transportFromAddr(addr net.Addr) string {
	switch addr.(type) {
	case *net.UDPAddr:
		return "udp"
	case *net.TCPAddr:
		return "tcp"
	default:
		return "null"
	}
}
