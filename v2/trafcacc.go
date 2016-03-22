// trafcacc.go traffic accelerate proxy exported functions

package trafcacc

import (
	"sync"
	"time"
)

//log "github.com/Sirupsen/logrus"

const (
	buffersize = 4096 * 2
	mtu        = buffersize - 100
	keepalive  = time.Second * 30
)

const (
	tcp = "tcp"
	udp = "udp"
)

var (
	udpBufferPool = &sync.Pool{New: func() interface{} { return make([]byte, buffersize) }}
)
