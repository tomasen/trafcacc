// trafcacc.go traffic accelerate proxy exported functions

package trafcacc

import (
	"net"
	"time"
)

//log "github.com/Sirupsen/logrus"

const (
	buffersize = 4096 * 2
	mtu        = buffersize - 100
	keepalive  = time.Second * 30
	rqudelay   = time.Millisecond * 500
)

const (
	tcp = "tcp"
	udp = "udp"
)

// Dialer TODO: comment
type Dialer interface {
	Setup(string)
	Dial() (net.Conn, error)
	DialTimeout(timeout time.Duration) (net.Conn, error)
	streampool() *streampool
}

// NewDialer TODO: comment
func NewDialer() Dialer {
	return newDialer()
}

// Handler TODO: comment
type Handler interface {
	Serve(net.Conn)
}

// Serve TODO: comment
type Serve interface {
	HandleFunc(listento string, handler func(net.Conn))
	Handle(listento string, handler Handler)
}
