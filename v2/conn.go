package trafcacc

import (
	"bytes"
	"io"
	"net"
	"sync/atomic"
	"time"
)

// packet conn
type pconn interface {
	pq() *packetQueue
	write(*packet) error
	role() string
}

// conn
type packetconn struct {
	pconn
	senderid uint32
	connid   uint32
	seqid    uint32
	rdr      bytes.Buffer
}

// Read reads data from the connection.
// Read can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (c *packetconn) Read(b []byte) (n int, err error) {
	if c.rdr.Len() > 0 {
		return c.rdr.Read(b)
	}

	c.pq().waitforArrived(c.senderid, c.connid)

	for {
		p := c.pq().pop(c.senderid, c.connid)
		if p == nil {
			break
		}
		// buffered reader writer
		_, err := c.rdr.Write(p.Buf)
		if err != nil {
			// TODO: deal with ErrTooLarge
		}

		if c.rdr.Len() > buffersize {
			break
		}
	}

	if c.pq().isClosed(c.senderid, c.connid) && c.rdr.Len() <= 0 {
		return 0, io.EOF
	}

	return c.rdr.Read(b)
}

// Write writes data to the connection.
// Write can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetWriteDeadline.
func (c *packetconn) Write(b []byte) (n int, err error) {
	// TODO: You can not send messages (datagrams) larger than 2^16 65536 octets with UDP.
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
func (c *packetconn) Close() error {

	cmd := closed
	if c.role() == "dialer" {
		cmd = close
	}
	err := c.write(&packet{
		Senderid: c.senderid,
		Seqid:    atomic.AddUint32(&c.seqid, 1),
		Connid:   c.connid,
		Cmd:      cmd,
	})

	// TODO: unblock read and write and return errors

	c.pq().close(c.senderid, c.connid)
	return err
}

// LocalAddr returns the local network address.
func (c *packetconn) LocalAddr() net.Addr {
	// TODO:
	return nil
}

// RemoteAddr returns the remote network address.
func (c *packetconn) RemoteAddr() net.Addr {
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
func (c *packetconn) SetDeadline(t time.Time) error {
	// TODO:
	return nil
}

// SetReadDeadline sets the deadline for future Read calls.
// A zero value for t means Read will not time out.
func (c *packetconn) SetReadDeadline(t time.Time) error {
	// TODO:
	return nil
}

// SetWriteDeadline sets the deadline for future Write calls.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (c *packetconn) SetWriteDeadline(t time.Time) error {
	// TODO:
	return nil
}
