package trafcacc

import (
	"bytes"
	"io"
	"net"
	"sync/atomic"
	"time"
)

// client side conn
type dialerConn struct {
	*dialer
	connid uint32
	seqid  uint32
	rdr    bytes.Buffer
}

// Read reads data from the connection.
// Read can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (d *dialerConn) Read(b []byte) (n int, err error) {

	d.pqd.waitforArrived(d.identity, d.connid)
	for {
		p := d.pqd.pop(d.identity, d.connid)
		if p == nil {
			break
		}
		// buffered reader writer
		d.rdr.Write(p.Buf)
	}

	if d.pqd.isClosed(d.identity, d.connid) && d.rdr.Len() == 0 {
		return 0, io.EOF
	}

	return d.rdr.Read(b)
}

// Write writes data to the connection.
// Write can be made to time out and return a Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetWriteDeadline.
func (d *dialerConn) Write(b []byte) (n int, err error) {
	err = d.write(&packet{
		Seqid:  atomic.AddUint32(&d.seqid, 1),
		Connid: d.connid,
		Buf:    b,
	})
	if err == nil {
		n = len(b)
	}
	return n, err
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (d *dialerConn) Close() error {
	err := d.write(&packet{
		Seqid:  atomic.AddUint32(&d.seqid, 1),
		Connid: d.connid,
		Cmd:    close,
	})

	// TODO: unblock read and write and return errors
	d.pqd.close(d.identity, d.connid)

	return err
}

// LocalAddr returns the local network address.
func (d *dialerConn) LocalAddr() net.Addr {
	// TODO:
	return nil
}

// RemoteAddr returns the remote network address.
func (d *dialerConn) RemoteAddr() net.Addr {
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
func (d *dialerConn) SetDeadline(t time.Time) error {
	// TODO:
	return nil
}

// SetReadDeadline sets the deadline for future Read calls.
// A zero value for t means Read will not time out.
func (d *dialerConn) SetReadDeadline(t time.Time) error {
	// TODO:
	return nil
}

// SetWriteDeadline sets the deadline for future Write calls.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (d *dialerConn) SetWriteDeadline(t time.Time) error {
	// TODO:
	return nil
}
