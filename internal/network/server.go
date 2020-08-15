//go:generate go run golang.org/x/tools/cmd/stringer -type=Transport

package network

import (
	"context"
	"fmt"
	"net"
	"time"

	"dotproxy/internal/metrics"
)

// contextKey is a type alias for context keys passed to server handlers.
type contextKey int

// Transport describes a network transport type.
type Transport int

// ServerHandler is a common interface that wraps logic for handling incoming connections on any
// network transport.
type ServerHandler interface {
	// Handle describes the routine to run when the server establishes a successful connection
	// with a client. The passed conn is a net.Conn-implementing TCPConn or UDPConn.
	Handle(ctx context.Context, conn net.Conn) error

	// ConsumeError is a callback invoked when the server fails to establish a connection with a
	// client, or when the handler returns an error.
	ConsumeError(ctx context.Context, err error)
}

// UDPServer describes a server that listens on a UDP address.
type UDPServer struct {
	addr string
	opts UDPServerOpts
}

// UDPServerOpts formalizes UDP server configuration options.
type UDPServerOpts struct {
	// MaxConcurrentConnections configures the maximum number of concurrent clients that the
	// server is capable of serving. It is generally recommended to set this value to the
	// highest number of concurrent connections the server can expect to receive, but it is safe
	// to set it lower.
	MaxConcurrentConnections int
	// ReadTimeout is the maximum amount of time the server will wait to read from a client.
	// Note that, since UDP is a connectionless protocol, this timeout value represents the
	// duration of time between when the socket begins listening for a connection to when the
	// client starts writing data.
	ReadTimeout time.Duration
	// WriteTimeout is the maximum amount of time the server is allowed to take to write data
	// back to a client, after which the server will consider the write to have failed.
	WriteTimeout time.Duration
}

// TCPServer describes a server that listens on a TCP address.
type TCPServer struct {
	addr   string
	cxHook metrics.ConnectionLifecycleHook
	opts   TCPServerOpts
}

// TCPServerOpts formalizes TCP server configuration options.
type TCPServerOpts struct {
	// ReadTimeout is the maximum amount of time the server will wait to read from a client
	// after it has established a connection with the server, after which the server will
	// consider the read to have failed.
	ReadTimeout time.Duration
	// WriteTimeout is the maximum amount of time the server is allowed to take to write to a
	// client, after which the server will consider the write to have failed.
	WriteTimeout time.Duration
}

const (
	// TransportContextKey is the name of the context key used to indicate the network transport
	// protocol the handler is serving. This is necessary because the handler APIs are
	// abstracted to the point that they are inherently agnostic to the client connection's
	// underlying transport.
	TransportContextKey contextKey = iota
)

const (
	// TCP describes a TCP transport.
	TCP Transport = iota
	// UDP describes a UDP transport.
	UDP
)

// NewUDPServer creates a UDP server listening on the specified address.
func NewUDPServer(addr string, opts UDPServerOpts) *UDPServer {
	// Sane option defaults
	if opts.MaxConcurrentConnections <= 0 {
		opts.MaxConcurrentConnections = 16
	}

	return &UDPServer{addr, opts}
}

// ListenAndServe starts listening on the UDP address with which the server was configured and
// indefinitely serves connections using the specified handler. It returns an error if it fails to
// bind to the initialized address.
func (s *UDPServer) ListenAndServe(handler ServerHandler) error {
	conn, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		return fmt.Errorf("server: failed to listen on UDP socket: err=%v", err)
	}

	ctx := context.WithValue(context.Background(), TransportContextKey, UDP)

	for i := 0; i < s.opts.MaxConcurrentConnections; i++ {
		go func() {
			for {
				udpConn := NewUDPConn(conn, s.opts.ReadTimeout, s.opts.WriteTimeout)

				if err := handler.Handle(ctx, udpConn); err != nil {
					handler.ConsumeError(ctx, err)
				}
			}
		}()
	}

	return nil
}

// NewTCPServer creates a TCP server listening on the specified address.
func NewTCPServer(addr string, cxHook metrics.ConnectionLifecycleHook, opts TCPServerOpts) *TCPServer {
	return &TCPServer{addr, cxHook, opts}
}

// ListenAndServe starts listening on the TCP address with which the server was configured and
// indefinitely serves connections using the specified handler. It returns an error if it fails to
//// bind to the initialized address.
func (s *TCPServer) ListenAndServe(handler ServerHandler) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("server: failed to listen on TCP socket: err=%v", err)
	}

	ctx := context.WithValue(context.Background(), TransportContextKey, TCP)

	for {
		conn, err := ln.Accept()
		if err != nil {
			s.cxHook.EmitConnectionError()
			handler.ConsumeError(ctx, err)
			continue
		}

		tcpConn := NewTCPConn(conn, s.opts.ReadTimeout, s.opts.WriteTimeout)
		s.cxHook.EmitConnectionOpen(0, tcpConn.RemoteAddr())

		go func() {
			defer func() {
				s.cxHook.EmitConnectionClose(tcpConn.RemoteAddr())
				tcpConn.Close()
			}()

			if err := handler.Handle(ctx, tcpConn); err != nil {
				handler.ConsumeError(ctx, err)
			}
		}()
	}
}
