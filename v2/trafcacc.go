// trafcacc.go traffic accelerate proxy exported functions

package trafcacc

import (
	"net"
	"time"
	//log "github.com/Sirupsen/logrus"
)

const (
	buffersize  = 4096 * 4
	dialtimeout = 15
	readtimeout = 30
)

func Dial(backend string) (net.Conn, error) {
	D := &Dialer{}
	return D.Dial(backend)
}

// DialTimeout acts like net.DialTimeout but take trafcacc addresses that backend listen to.
func DialTimeout(backend string, timeout time.Duration) (net.Conn, error) {
	D := &Dialer{Timeout: timeout}
	return D.Dial(backend)
}

// HandleFunc registers the handler for the given addresses
// that back-end server listened to
func HandleFunc(listento string, handler func(net.Conn)) {
	DefaultServeMux.Handle(listento, HandlerFunc(handler))
}

// Handle registers the handler for the given addresses
func Handle(listento string, handler Handler) {
	DefaultServeMux.Handle(listento, handler)
}

// Accelerate traffic by setup front-end dialer and back-end server
func Accelerate(l, u string, role tag) Trafcacc {

	t := &trafcacc{role: role}
	t.accelerate(l, u)

	return t
}
