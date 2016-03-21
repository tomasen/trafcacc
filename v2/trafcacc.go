// trafcacc.go traffic accelerate proxy exported functions

package trafcacc

import "time"

//log "github.com/Sirupsen/logrus"

const (
	buffersize = 4096 * 16
	keepalive  = time.Second * 30

	//dialtimeout = 15
	//readtimeout = 30
)

const (
	tcp = "tcp"
	udp = "udp"
)
