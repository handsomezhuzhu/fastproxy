package proxy

import (
	"bufio"
	"io"
	"net"
	"sync/atomic"
	"time"
)

// clientConn reads through the buffered reader (which may already hold
// buffered bytes) and writes straight to the underlying connection.
type clientConn struct {
	br *bufio.Reader
	net.Conn
}

func (c *clientConn) Read(p []byte) (int, error) { return c.br.Read(p) }

// CloseWrite must be explicit: the embedded net.Conn interface does not
// promote CloseWrite from the concrete connection underneath.
func (c *clientConn) CloseWrite() error {
	if hc, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return hc.CloseWrite()
	}
	return nil
}

// bufferedConn is an upstream connection where a handshake reader may have
// consumed extra bytes beyond the handshake.
type bufferedConn struct {
	br *bufio.Reader
	net.Conn
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.br.Read(p) }

func (c *bufferedConn) CloseWrite() error {
	if hc, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return hc.CloseWrite()
	}
	return nil
}

// countedConn meters client<->server traffic into the port meter, the account
// meter (once authenticated) and the per-connection counters used for the
// access log.
// The meter type (byte counters plus connection counters) lives in manager.go.
type countedConn struct {
	net.Conn
	port      *meter
	acc       *meter
	lin, lout *atomic.Int64
}

func (c *countedConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.port.in.Add(int64(n))
		if c.acc != nil {
			c.acc.in.Add(int64(n))
		}
		if c.lin != nil {
			c.lin.Add(int64(n))
		}
	}
	return n, err
}

func (c *countedConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.port.out.Add(int64(n))
		if c.acc != nil {
			c.acc.out.Add(int64(n))
		}
		if c.lout != nil {
			c.lout.Add(int64(n))
		}
	}
	return n, err
}

// CloseWrite promotes a TCP half-close so remote peers observe EOF.
func (c *countedConn) CloseWrite() error {
	if hc, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return hc.CloseWrite()
	}
	return nil
}

// relay pipes both directions until both close. When one direction finishes,
// the other gets a TCP half-close so the remote peer sees EOF while the
// remaining direction can still flush data. A safety timer guarantees the
// connection is fully closed if the final direction never ends.
func relay(a, b io.ReadWriteCloser) {
	fin := make(chan struct{}, 2)
	go func() {
		io.Copy(a, b) // upstream -> client
		halfClose(a)
		fin <- struct{}{}
	}()
	go func() {
		io.Copy(b, a) // client -> upstream
		halfClose(b)
		fin <- struct{}{}
	}()
	<-fin
	t := time.NewTimer(2 * time.Minute)
	defer t.Stop()
	select {
	case <-fin:
	case <-t.C:
	}
	a.Close()
	b.Close()
}

func halfClose(c io.Closer) {
	if hc, ok := c.(interface{ CloseWrite() error }); ok {
		hc.CloseWrite()
	}
}
