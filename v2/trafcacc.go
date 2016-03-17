// trafcacc.go traffic accelerate proxy exported functions

package trafcacc

import "net"

//log "github.com/Sirupsen/logrus"

const (
	buffersize  = 4096 * 4
	dialtimeout = 15
	readtimeout = 30
)

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
