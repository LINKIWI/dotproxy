package network

import (
	"fmt"
	"net"
	"time"
)

// UDPConn is an abstraction over a UDP net.PacketConn to give it net.Conn-like semantics. It
// statefully tracks connection state changes across reads and writes, assuming that a write follows
// an initial read.
type UDPConn struct {
	conn         net.PacketConn
	readTimeout  time.Duration
	writeTimeout time.Duration
	remote       net.Addr
}

// TCPConn is an abstraction over a net.Conn that provides dynamic read and write timeouts.
type TCPConn struct {
	readTimeout  time.Duration
	writeTimeout time.Duration

	net.Conn
}

// NewUDPConn creates a UDPConn from a backing net.PacketConn.
func NewUDPConn(conn net.PacketConn, readTimeout time.Duration, writeTimeout time.Duration) *UDPConn {
	return &UDPConn{
		conn:         conn,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}
}

// Read performs a read from the remote client. The remote address is statefully tracked as a struct
// member.
func (c *UDPConn) Read(buf []byte) (n int, err error) {
	if c.remote != nil {
		return 0, fmt.Errorf("conn: already associated with a transaction")
	}

	if c.readTimeout > 0 {
		if err := c.SetReadDeadline(time.Now().Add(c.readTimeout)); err != nil {
			return 0, err
		}
	}

	n, c.remote, err = c.conn.ReadFrom(buf)

	return
}

// Write writes to the same client from which data was read. It is an error to write to a connection
// without a prior read from a remote client.
func (c *UDPConn) Write(buf []byte) (n int, err error) {
	if c.remote == nil {
		return 0, fmt.Errorf("conn: no remote associated with this connection")
	}

	if c.writeTimeout > 0 {
		if err := c.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
			return 0, err
		}
	}

	return c.conn.WriteTo(buf, c.remote)
}

// Close closes the underlying connection.
func (c *UDPConn) Close() error {
	return c.conn.Close()
}

// LocalAddr obtains the connection's local address.
func (c *UDPConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr obtains the connection's remote address.
func (c *UDPConn) RemoteAddr() net.Addr {
	return c.remote
}

// SetDeadline sets both the read and write deadline.
func (c *UDPConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

// SetReadDeadline sets the read deadline.
func (c *UDPConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline.
func (c *UDPConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// NewTCPConn creates a TCPConn from a backing net.Conn.
func NewTCPConn(conn net.Conn, readTimeout time.Duration, writeTimeout time.Duration) *TCPConn {
	return &TCPConn{
		Conn:         conn,
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}
}

// Read sets a read deadline followed by reading from the backing connection.
func (c *TCPConn) Read(buf []byte) (n int, err error) {
	if c.readTimeout > 0 {
		if err := c.SetReadDeadline(time.Now().Add(c.readTimeout)); err != nil {
			return 0, err
		}
	}

	return c.Conn.Read(buf)
}

// Write sets a write deadline followed by reading from the backing connection.
func (c *TCPConn) Write(buf []byte) (n int, err error) {
	if c.writeTimeout > 0 {
		if err := c.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
			return 0, err
		}
	}

	return c.Conn.Write(buf)
}
