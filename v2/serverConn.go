package trafcacc

import (
	"bytes"
	"io"
	"net"
	"sync/atomic"
	"time"
)

// client side conn
type serverConn struct {
	*ServeMux
	senderid uint32
	connid   uint32
	seqid    uint32
	rdr      bytes.Buffer
}

// Read reads data from the connection.
// Read can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (c *serverConn) Read(b []byte) (n int, err error) {

	cond := c.packetQueue.cond(c.senderid, c.connid)
	if cond == nil {
		if c.rdr.Len() > 0 {
			return c.rdr.Read(b)
		}
		return 0, io.EOF
	}
	cond.L.Lock()
	for !c.packetQueue.arrived(c.senderid, c.connid) {
		cond.Wait()
	}
	for {
		p := c.packetQueue.pop(c.senderid, c.connid)
		if p == nil {
			break
		}
		// buffered reader writer
		c.rdr.Write(p.Buf)
	}
	cond.L.Unlock()
	cond.Broadcast()

	return c.rdr.Read(b)
}

// Write writes data to the connection.
// Write can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetWriteDeadline.
func (c *serverConn) Write(b []byte) (n int, err error) {
	err = c.write(&packet{
		Senderid: c.senderid,
		Seqid:    atomic.AddUint32(&c.seqid, 1),
		Connid:   c.connid,
		Buf:      b,
	})
	if err == nil {
		n = len(b)
	}
	return n, err
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (c *serverConn) Close() error {
	err := c.write(&packet{
		Senderid: c.senderid,
		Seqid:    atomic.AddUint32(&c.seqid, 1),
		Connid:   c.connid,
		Cmd:      closed,
	})

	// TODO: unblock read and write and return errors

	go c.packetQueue.close(c.senderid, c.connid)

	return err
}

// LocalAddr returns the local network address.
func (c *serverConn) LocalAddr() net.Addr {
	// TODO:
	return nil
}

// RemoteAddr returns the remote network address.
func (c *serverConn) RemoteAddr() net.Addr {
	// TODO:
	return nil
}

// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
//
// A deadline is an absolute time after which I/O operations
// fail with a timeout (see type Error) instead of
// blocking. The deadline applies to all future I/O, not just
// the immediately following call to Read or Write.
//
// An idle timeout can be implemented by repeatedly extending
// the deadline after successful Read or Write calls.
//
// A zero value for t means I/O operations will not time out.
func (c *serverConn) SetDeadline(t time.Time) error {
	// TODO:
	return nil
}

// SetReadDeadline sets the deadline for future Read calls.
// A zero value for t means Read will not time out.
func (c *serverConn) SetReadDeadline(t time.Time) error {
	// TODO:
	return nil
}

// SetWriteDeadline sets the deadline for future Write calls.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (c *serverConn) SetWriteDeadline(t time.Time) error {
	// TODO:
	return nil
}
