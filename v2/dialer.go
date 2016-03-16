package trafcacc

import (
	"net"
	"time"
)

type Dialer struct {
	// Timeout is the maximum amount of time a dial will wait for
	// a connect to complete. If Deadline is also set, it may fail
	// earlier.
	//
	// The default is no timeout.
	//
	// When dialing a name with multiple IP addresses, the timeout
	// may be divided between them.
	//
	// With or without a timeout, the operating system may impose
	// its own earlier timeout. For instance, TCP timeouts are
	// often around 3 minutes.
	Timeout time.Duration

	// Deadline is the absolute point in time after which dials
	// will fail. If Timeout is set, it may fail earlier.
	// Zero means no deadline, or dependent on the operating system
	// as with the Timeout option.
	Deadline time.Time

	// LocalAddr is the local address to use when dialing an
	// address. The address must be of a compatible type for the
	// network being dialed.
	// If nil, a local address is automatically chosen.
	LocalAddr net.Addr

	// DualStack enables RFC 6555-compliant "Happy Eyeballs" dialing
	// when the network is "tcp" and the destination is a host name
	// with both IPv4 and IPv6 addresses. This allows a client to
	// tolerate networks where one address family is silently broken.
	DualStack bool

	// FallbackDelay specifies the length of time to wait before
	// spawning a fallback connection, when DualStack is enabled.
	// If zero, a default delay of 300ms is used.
	FallbackDelay time.Duration

	// KeepAlive specifies the keep-alive period for an active
	// network connection.
	// If zero, keep-alives are not enabled. Network protocols
	// that do not support keep-alives ignore this field.
	KeepAlive time.Duration

	// Cancel is an optional channel whose closure indicates that
	// the dial should be canceled. Not all types of dials support
	// cancelation.
	Cancel <-chan struct{}
}

func (d *Dialer) Dial(address string) (net.Conn, error) {
	// TODO:
	return nil, nil
}
