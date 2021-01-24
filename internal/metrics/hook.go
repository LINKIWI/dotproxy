package metrics

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"lib.kevinlin.info/aperture"
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
	// EmitRead reports a successful connection read.
	EmitRead(latency time.Duration, addr net.Addr)

	// EmitReadError reports the event that a connection read failed.
	EmitReadError(addr net.Addr)

	// EmitWrite reports a successful connection write.
	EmitWrite(latency time.Duration, addr net.Addr)

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

	// EmitProcess reports the occurrence of a processed proxy request.
	EmitProcess(client net.Addr, upstream net.Addr)

	// EmitError reports the occurrence of a critical error in the proxy lifecycle that causes
	// the request to not be correctly served.
	EmitError()
}

// AsyncStatsdConnectionLifecycleHook is an implementation of ConnectionLifecycleHook that outputs
// metrics asynchronously to statsd.
type AsyncStatsdConnectionLifecycleHook struct {
	client aperture.Statsd
	source string
}

// AsyncStatsdConnectionIOHook is an implementation of ConnectionIOHook that outputs metrics
// asynchronously to statsd.
type AsyncStatsdConnectionIOHook struct {
	client aperture.Statsd
	source string
}

// AsyncStatsdProxyHook is an implementation of ProxyHook that outputs metrics asynchronously to
// statsd.
type AsyncStatsdProxyHook struct {
	client     aperture.Statsd
	sequenceID int64
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
func NewAsyncStatsdConnectionLifecycleHook(source string, addr string, sampleRate float64, version string) (ConnectionLifecycleHook, error) {
	client, err := statsdClientFactory(addr, sampleRate, version)
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
		tags := map[string]interface{}{
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
	go h.client.Count(fmt.Sprintf("event.%s.cx_close", h.source), 1, map[string]interface{}{
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
func NewAsyncStatsdConnectionIOHook(source string, addr string, sampleRate float64, version string) (ConnectionIOHook, error) {
	client, err := statsdClientFactory(addr, sampleRate, version)
	if err != nil {
		return nil, err
	}

	return &AsyncStatsdConnectionIOHook{
		client: client,
		source: source,
	}, nil
}

// EmitRead statsd implementation.
func (h *AsyncStatsdConnectionIOHook) EmitRead(latency time.Duration, addr net.Addr) {
	go func() {
		tags := map[string]interface{}{
			"addr":      ipFromAddr(addr),
			"transport": transportFromAddr(addr),
		}

		h.client.Count(fmt.Sprintf("event.%s.cx_read", h.source), 1, tags)
		h.client.Timing(fmt.Sprintf("latency.%s.cx_read", h.source), latency, tags)
	}()
}

// EmitReadError statsd implementation.
func (h *AsyncStatsdConnectionIOHook) EmitReadError(addr net.Addr) {
	go h.client.Count(fmt.Sprintf("event.%s.cx_read_error", h.source), 1, map[string]interface{}{
		"addr":      ipFromAddr(addr),
		"transport": transportFromAddr(addr),
	})
}

// EmitWrite statsd implementation.
func (h *AsyncStatsdConnectionIOHook) EmitWrite(latency time.Duration, addr net.Addr) {
	go func() {
		tags := map[string]interface{}{
			"addr":      ipFromAddr(addr),
			"transport": transportFromAddr(addr),
		}

		h.client.Count(fmt.Sprintf("event.%s.cx_write", h.source), 1, tags)
		h.client.Timing(fmt.Sprintf("latency.%s.cx_write", h.source), latency, tags)
	}()
}

// EmitWriteError statsd implementation.
func (h *AsyncStatsdConnectionIOHook) EmitWriteError(addr net.Addr) {
	go h.client.Count(fmt.Sprintf("event.%s.cx_write_error", h.source), 1, map[string]interface{}{
		"addr":      ipFromAddr(addr),
		"transport": transportFromAddr(addr),
	})
}

// EmitRetry statsd implementation.
func (h *AsyncStatsdConnectionIOHook) EmitRetry(addr net.Addr) {
	go h.client.Count(fmt.Sprintf("event.%s.cx_io_retry", h.source), 1, map[string]interface{}{
		"addr":      ipFromAddr(addr),
		"transport": transportFromAddr(addr),
	})
}

// NewNoopConnectionIOHook creates a noop implementation of ConnectionIOHook.
func NewNoopConnectionIOHook() ConnectionIOHook {
	return &NoopConnectionIOHook{}
}

// EmitRead noops.
func (h *NoopConnectionIOHook) EmitRead(latency time.Duration, addr net.Addr) {}

// EmitReadError noops.
func (h *NoopConnectionIOHook) EmitReadError(addr net.Addr) {}

// EmitWrite noops.
func (h *NoopConnectionIOHook) EmitWrite(latency time.Duration, addr net.Addr) {}

// EmitWriteError noops.
func (h *NoopConnectionIOHook) EmitWriteError(addr net.Addr) {}

// EmitRetry noops.
func (h *NoopConnectionIOHook) EmitRetry(addr net.Addr) {}

// NewAsyncStatsdProxyHook creates a new client with the specified statsd address and sample rate.
func NewAsyncStatsdProxyHook(addr string, sampleRate float64, version string) (ProxyHook, error) {
	client, err := statsdClientFactory(addr, sampleRate, version)
	if err != nil {
		return nil, err
	}

	return &AsyncStatsdProxyHook{client: client}, nil
}

// EmitRequestSize statsd implementation
func (h *AsyncStatsdProxyHook) EmitRequestSize(bytes int64, client net.Addr) {
	go h.client.Size("size.proxy.request", bytes, map[string]interface{}{
		"addr": ipFromAddr(client),
	})
}

// EmitResponseSize statsd implementation
func (h *AsyncStatsdProxyHook) EmitResponseSize(bytes int64, upstream net.Addr) {
	go h.client.Size("size.proxy.response", bytes, map[string]interface{}{
		"addr": ipFromAddr(upstream),
	})
}

// EmitRTT statsd implementation
func (h *AsyncStatsdProxyHook) EmitRTT(latency time.Duration, client net.Addr, upstream net.Addr) {
	go h.client.Timing("latency.proxy.tx_rtt", latency, map[string]interface{}{
		"client":    ipFromAddr(client),
		"upstream":  ipFromAddr(upstream),
		"transport": transportFromAddr(client),
	})
}

// EmitUpstreamLatency statsd implementation
func (h *AsyncStatsdProxyHook) EmitUpstreamLatency(latency time.Duration, client net.Addr, upstream net.Addr) {
	go h.client.Timing("latency.proxy.tx_upstream", latency, map[string]interface{}{
		"client":   ipFromAddr(client),
		"upstream": ipFromAddr(upstream),
	})
}

// EmitProcess statsd implementation
func (h *AsyncStatsdProxyHook) EmitProcess(client net.Addr, upstream net.Addr) {
	go func() {
		tags := map[string]interface{}{
			"client":   ipFromAddr(client),
			"upstream": ipFromAddr(upstream),
		}

		h.client.Count("event.proxy.process", 1, tags)
		h.client.Gauge(
			"gauge.proxy.sequence_id",
			float64(atomic.LoadInt64(&h.sequenceID)),
			tags,
		)

		atomic.AddInt64(&h.sequenceID, 1)
	}()
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

// EmitProcess noops.
func (h *NoopProxyHook) EmitProcess(client net.Addr, upstream net.Addr) {}

// EmitError noops.
func (h *NoopProxyHook) EmitError() {}

// statsdClientFactory creates a configured statsd client with reasonable defaults for the given
// statsd server address and sample rate.
func statsdClientFactory(addr string, sampleRate float64, version string) (*aperture.Client, error) {
	return aperture.NewClient(&aperture.Config{
		Address:                addr,
		Prefix:                 "dotproxy",
		SampleRate:             sampleRate,
		TransportProbeInterval: 10 * time.Second,
		DefaultTags: map[string]interface{}{
			"version": version,
		},
	})
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
