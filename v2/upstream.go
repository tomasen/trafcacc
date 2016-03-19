package trafcacc

import (
	"encoding/gob"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
)

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

type streampool struct {
	pool []*upstream
	cond *sync.Cond
}

func newStreamPool() *streampool {
	return &streampool{
		// TODO: use RWMutex maybe?
		cond: sync.NewCond(&sync.Mutex{}),
	}
}

func (pool *streampool) append(u *upstream) {
	pool.cond.L.Lock()
	pool.pool = append(pool.pool, u)
	pool.cond.L.Unlock()
	pool.cond.Broadcast()
}

func (pool *streampool) pickupstreams() []*upstream {
	pool.cond.L.Lock()
	defer pool.cond.L.Unlock()
	var alived []*upstream
	for _, v := range pool.pool {
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
		return alived
	default:
		idx := rand.Intn(length)
		return []*upstream{alived[idx], alived[(idx+1)%length]}
	}
}

// check if there is any alive upstream
func (pool *streampool) alive() bool {
	for _, v := range pool.pool {
		if v.proto == "udp" {
			return true
		}
		if time.Now().Sub(v.alive) < keepalive {
			return true
		}
	}
	return false
}

func (pool *streampool) waitforalive() {
	pool.cond.L.Lock()
	for !pool.alive() {
		pool.cond.Wait()
	}
	pool.cond.L.Unlock()
}

func (pool *streampool) remove(proto, addr string) {
	pool.cond.L.Lock()
	for k, v := range pool.pool {
		if v.proto == proto && v.addr == addr {
			pool.pool = append(pool.pool[:k], pool.pool[k+1:]...)
		}
	}
	pool.cond.L.Unlock()
	pool.cond.Broadcast()
}
