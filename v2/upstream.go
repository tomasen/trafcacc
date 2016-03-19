package trafcacc

import (
	"encoding/gob"
	"math/rand"
	"net"
	"time"

	"github.com/Sirupsen/logrus"
)

type streampool []*upstream

type upstream struct {
	proto string
	addr  string

	conn    net.Conn
	encoder *gob.Encoder
	decoder *gob.Decoder

	alive time.Time
	// mux     sync.RWMutex
}

func (u *upstream) ping() error {
	err := u.encoder.Encode(&packet{Cmd: ping})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Warnln("Dialer ping upstream error")
	}
	return err
}

func (pool streampool) pickupstreams() []*upstream {

	var alived []*upstream
	for _, v := range pool {
		if keepalive > time.Now().Sub(v.alive) {
			alived = append(alived, v)
		}
	}
	// avoid duplicate
	length := len(alived)
	switch length {
	case 0:
		return nil
	case 1:
		return pool
	default:
		idx := rand.Intn(length)
		return []*upstream{alived[idx], alived[(idx+1)%length]}
	}
}

// check if there is any alive upstream
func (pool streampool) alive() bool {
	for _, v := range pool {
		if v.proto == "udp" {
			return true
		}
		if time.Now().Sub(v.alive) < keepalive {
			return true
		}
	}
	return false
}

func (pool streampool) remove(proto, addr string) {
	for k, v := range pool {
		if v.proto == proto && v.addr == addr {
			pool = append(pool[:k], pool[k+1:]...)
		}
	}
}
