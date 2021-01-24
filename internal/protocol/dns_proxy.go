package protocol

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/getsentry/raven-go"
	"lib.kevinlin.info/aperture/lib"

	"dotproxy/internal/log"
	"dotproxy/internal/metrics"
	"dotproxy/internal/network"
)

// DNSProxyHandler is a semi-DNS-protocol-aware server handler that proxies requests between a
// client and upstream server.
type DNSProxyHandler struct {
	Upstream         network.Client
	ClientCxIOHook   metrics.ConnectionIOHook
	UpstreamCxIOHook metrics.ConnectionIOHook
	ProxyHook        metrics.ProxyHook
	Logger           log.Logger
	Opts             DNSProxyOpts
}

// DNSProxyOpts formalizes configuration options for the proxy handler.
type DNSProxyOpts struct {
	// MaxUpstreamRetries describes the maximum allowable times the proxy server is permitted to
	// retry a request with the upstream server(s). It is recommended to set this to a liberal
	// value above 0; since connections are pooled and persisted over a long period of time, it
	// is highly likely that any single proxy request will fail (due to a server-side closed
	// connection) and will need to be retried with another connection in the pool.
	MaxUpstreamRetries int
}

// ConsumeError simply logs the proxy error.
func (h *DNSProxyHandler) ConsumeError(ctx context.Context, err error) {
	h.Logger.Error("%v", err)
	h.ProxyHook.EmitError()

	raven.CaptureError(err, map[string]string{
		"transport": ctx.Value(network.TransportContextKey).(network.Transport).String(),
	})
}

// Handle reads a request from the client connection, writes the request to the upstream connection,
// reads the response from the upstream connection, and finally writes the response back to the
// client. It performs some minimal protocol-aware data shaping and emits metrics along the way.
func (h *DNSProxyHandler) Handle(ctx context.Context, clientConn net.Conn) error {
	rttTxTimer := lib.NewStopwatch()

	/* Read the DNS request from the client */

	clientReq, err := h.clientRead(clientConn)
	if err != nil {
		return err
	}

	h.Logger.Debug(
		"dns_proxy: read request from client: request_bytes=%d transport=%s",
		len(clientReq),
		ctx.Value(network.TransportContextKey),
	)

	if ctx.Value(network.TransportContextKey) == network.UDP {
		// Since UDP is connectionless, the initial network read blocks until data is
		// available. Reset the RTT timer here to get an approximately correct estimate of
		// end-to-end latency.
		rttTxTimer = lib.NewStopwatch()

		// By RFC specification, DNS over TCP transports should include a two-octet header
		// in the request that denotes the size of the DNS packet. Since this request came
		// in on a UDP transport, augment the request payload to conform to standard.
		clientHeader := make([]byte, 2)
		binary.BigEndian.PutUint16(clientHeader, uint16(len(clientReq)))
		clientReq = append(clientHeader, clientReq...)
	}

	/* Open a (possibly cached) connection to the upstream and perform a W/R transaction */

	maxRetries := h.Opts.MaxUpstreamRetries
	if maxRetries <= 0 {
		maxRetries = 16
	}

	upstreamResp, upstreamConn, err := h.proxyUpstream(clientConn, clientReq, maxRetries)
	if err != nil {
		return err
	}

	// Omit the response's size header if the client initially requested a UDP transport
	if ctx.Value(network.TransportContextKey) == network.UDP {
		upstreamResp = upstreamResp[2:]
	}

	/* Write the proxied result back to the client */

	if err := h.clientWrite(clientConn, upstreamResp); err != nil {
		return err
	}

	h.Logger.Debug(
		"dns_proxy: completed write back to client: rtt=%v transport=%s",
		rttTxTimer.Elapsed(),
		ctx.Value(network.TransportContextKey),
	)

	/* Clean up and report end-to-end metrics */

	h.ProxyHook.EmitProcess(clientConn.RemoteAddr(), upstreamConn.RemoteAddr())
	h.ProxyHook.EmitRequestSize(int64(len(clientReq)), clientConn.RemoteAddr())
	h.ProxyHook.EmitResponseSize(int64(len(upstreamResp)), upstreamConn.RemoteAddr())
	h.ProxyHook.EmitRTT(
		rttTxTimer.Elapsed(),
		clientConn.RemoteAddr(),
		upstreamConn.RemoteAddr(),
	)

	return nil
}

// clientRead reads a request from the client.
func (h *DNSProxyHandler) clientRead(conn net.Conn) ([]byte, error) {
	clientReadTimer := lib.NewStopwatch()
	clientReq := make([]byte, 1024) // The DNS protocol limits the maximum size of a DNS packet.

	clientReadBytes, err := conn.Read(clientReq)
	if err != nil {
		h.ClientCxIOHook.EmitReadError(conn.RemoteAddr())
		return nil, fmt.Errorf("dns_proxy: error reading request from client: err=%v", err)
	}

	h.ClientCxIOHook.EmitRead(clientReadTimer.Elapsed(), conn.RemoteAddr())

	// Trim the request buffer to only what the server was able to read
	return clientReq[:clientReadBytes], nil
}

// upstreamTransact performs a write-read transaction with the upstream connection and returns the
// upstream response.
func (h *DNSProxyHandler) upstreamTransact(client net.Conn, upstream *network.PersistentConn, clientReq []byte) ([]byte, error) {
	upstreamTxTimer := lib.NewStopwatch()

	/* Proxy the client request to the upstream */

	upstreamWriteTimer := lib.NewStopwatch()

	upstreamWriteBytes, err := upstream.Write(clientReq)
	if err != nil || upstreamWriteBytes != len(clientReq) {
		h.UpstreamCxIOHook.EmitWriteError(upstream.RemoteAddr())
		return nil, fmt.Errorf("dns_proxy: error writing to upstream: err=%v", err)
	}

	h.UpstreamCxIOHook.EmitWrite(upstreamWriteTimer.Elapsed(), upstream.RemoteAddr())

	h.Logger.Debug("dns_proxy: wrote request to upstream: request_bytes=%d", upstreamWriteBytes)

	/* Read the response from the upstream */

	upstreamReadTimer := lib.NewStopwatch()

	// By RFC specification, the server response follows the same format as the TCP request: the
	// first two bytes specify the length of the message.
	upstreamHeader := make([]byte, 2)
	upstreamHeaderBytes, err := upstream.Read(upstreamHeader)
	if err != nil || upstreamHeaderBytes != 2 {
		h.UpstreamCxIOHook.EmitReadError(upstream.RemoteAddr())
		return nil, fmt.Errorf(
			"dns_proxy: error reading header from upstream: err=%v bytes=%d",
			err,
			upstreamHeaderBytes,
		)
	}

	// Parse the alleged size of the remaining response and perform another exactly-sized read.
	respSize := binary.BigEndian.Uint16(upstreamHeader)
	upstreamResp := make([]byte, respSize)

	h.Logger.Debug("dns_proxy: read upstream header: response_size=%d", respSize)

	upstreamReadBytes, err := upstream.Read(upstreamResp)
	if err != nil || upstreamReadBytes != int(respSize) {
		h.UpstreamCxIOHook.EmitReadError(upstream.RemoteAddr())
		return nil, fmt.Errorf(
			"dns_proxy: error reading full response from upstream: err=%v bytes=%d",
			err,
			upstreamReadBytes,
		)
	}

	h.Logger.Debug("dns_proxy: read upstream response: response_bytes=%d", upstreamReadBytes)

	h.UpstreamCxIOHook.EmitRead(upstreamReadTimer.Elapsed(), upstream.RemoteAddr())
	h.ProxyHook.EmitUpstreamLatency(
		upstreamTxTimer.Elapsed(),
		client.RemoteAddr(),
		upstream.RemoteAddr(),
	)

	return append(upstreamHeader, upstreamResp...), nil
}

// proxyUpstream opens an upstream connection and performs a write-read transaction with a client
// request, wrapping retry logic. It returns the upstream response, the upstream connection, and
// optionally an error.
func (h *DNSProxyHandler) proxyUpstream(client net.Conn, clientReq []byte, retries int) ([]byte, net.Conn, error) {
	upstream, err := h.Upstream.Conn()
	if err != nil {
		return nil, nil, fmt.Errorf(
			"dns_proxy: error opening upstream connection: err=%v",
			err,
		)
	}

	h.Logger.Debug("dns_proxy: created upstream connection: conn=%v", upstream)

	resp, err := h.upstreamTransact(client, upstream, clientReq)
	if err != nil {
		// No matter the retry budget, destroy the connection if it fails during I/O
		go upstream.Destroy()

		if retries > 0 {
			h.UpstreamCxIOHook.EmitRetry(upstream.RemoteAddr())
			h.Logger.Debug(
				"dns_proxy: upstream I/O failed; retrying: retry=%d",
				retries,
			)

			return h.proxyUpstream(client, clientReq, retries-1)
		}

		h.Logger.Debug("dns_proxy: upstream I/O failed; available retries exhausted")

		return nil, nil, err
	}

	// Upstream transaction succeeded; schedule the connection for reinsertion into the
	// long-lived connection pool
	go upstream.Close()

	h.Logger.Debug("dns_proxy: completed upstream proxy: response_bytes=%d", len(resp))

	return resp, upstream, err
}

// clientWrite writes data back to the client.
func (h *DNSProxyHandler) clientWrite(conn net.Conn, upstreamResp []byte) error {
	clientWriteTimer := lib.NewStopwatch()
	clientWriteBytes, err := conn.Write(upstreamResp)

	if err != nil {
		h.ClientCxIOHook.EmitWriteError(conn.RemoteAddr())
		return err
	}

	if clientWriteBytes != len(upstreamResp) {
		h.ClientCxIOHook.EmitWriteError(conn.RemoteAddr())
		return fmt.Errorf(
			"dns_proxy: failed writing response bytes to client: expected=%d actual=%d",
			len(upstreamResp),
			clientWriteBytes,
		)
	}

	h.ClientCxIOHook.EmitWrite(clientWriteTimer.Elapsed(), conn.RemoteAddr())

	return nil
}
