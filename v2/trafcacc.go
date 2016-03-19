// trafcacc.go traffic accelerate proxy exported functions

package trafcacc

import (
	"net"
	"time"
)

//log "github.com/Sirupsen/logrus"

const (
	buffersize = 4096 * 4
	keepalive  = time.Second * 3

	dialtimeout = 15
	readtimeout = 30
)

// Dialer TODO: comment
type Dialer interface {
	Setup(server string)
	Dial() (net.Conn, error)
	DialTimeout(timeout time.Duration) (net.Conn, error)
}

// Handle registers the handler for the given addresses
func Handle(listento string, handler Handler) {
	DefaultServeMux.Handle(listento, handler)
}

// HandleFunc registers the handler for the given addresses
// that back-end server listened to
func HandleFunc(listento string, handler func(net.Conn)) {
	Handle(listento, HandlerFunc(handler))
}

// Accelerate traffic by setup front-end dialer and back-end server
func Accelerate(l, u string, role tag) Trafcacc {

	t := &trafcacc{role: role}
	t.accelerate(l, u)

	return t
}
