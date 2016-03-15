// trafcacc.go 启动 traffic accelerate proxy

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

// DialTimeout acts like net.Dial but take trafcacc addresses that backend listen to.
func DialTimeout(backend string, timeout time.Duration) net.Conn {
	// TODO: dial as front-end
	return nil
}

// HandleFunc registers the handler for the given addresses
// that back-end server listened to
func HandleFunc(listento string, handler func(net.Conn)) {
	// TODO: handle as backend
}

// Accelerate traffic by setup front-end dialer and back-end server
// TODO: maybe not put this inside package
func Accelerate(l, u string, role tag) Trafcacc {

	t := &trafcacc{role: role}
	t.accelerate(l, u)

	return t
}
