package trafcacc

import "net"

type ServeMux struct {
	// contains filtered or unexported fields
}

type Handler interface {
	Serve(net.Conn)
}

type HandlerFunc func(net.Conn)

func (f HandlerFunc) Serve(c net.Conn) {
	f(c)
}

// NewServeMux allocates and returns a new ServeMux.
func NewServeMux() *ServeMux {
	return &ServeMux{}
}

var DefaultServeMux = NewServeMux()

// HandleFunc registers the handler for the given addresses
// that back-end server listened to
func (mux *ServeMux) HandleFunc(listento string, handler func(net.Conn)) {
	mux.Handle(listento, HandlerFunc(handler))
}

// Handle registers the handler for the given addresses
func (mux *ServeMux) Handle(listento string, handler Handler) {
	// TODO: handle as backend
}
