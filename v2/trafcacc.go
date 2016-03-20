// trafcacc.go traffic accelerate proxy exported functions

package trafcacc

import (
	"net"
	"time"
)

//log "github.com/Sirupsen/logrus"

const (
	buffersize = 4096 * 4
	keepalive  = time.Second * 30

	dialtimeout = 15
	readtimeout = 30
)

// Dialer TODO: comment
type Dialer interface {
	Setup(server string)
	Dial() (net.Conn, error)
	DialTimeout(timeout time.Duration) (net.Conn, error)
}

// Accelerate traffic by setup front-end dialer and back-end server
func Accelerate(l, u string, role tag) Trafcacc {

	t := &trafcacc{role: role}
	t.accelerate(l, u)

	return t
}
