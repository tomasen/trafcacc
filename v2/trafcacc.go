// trafcacc.go traffic accelerate proxy exported functions

package trafcacc

import (
	"time"
	"sync"
)

//log "github.com/Sirupsen/logrus"

const (
	buffersize = 65535
	mtu  			 = buffersize-50
	keepalive  = time.Second * 30

	//dialtimeout = 15
	//readtimeout = 30
)

const (
	tcp = "tcp"
	udp = "udp"
)

var (
	udpBufferPool     =  &sync.Pool{New:func()interface{}{return make([]byte, buffersize)}}
)
