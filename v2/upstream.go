package trafcacc

import (
	"bytes"
	"encoding/gob"
	"errors"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
)

type upstream struct {
	uuid  uint64
	proto string
	alive int64

	// tcp only
	encoder *gob.Encoder
	decoder *gob.Decoder

	// udp only (server)
	udpconn *net.UDPConn
	udpaddr *net.UDPAddr

	// dialer only
	conn net.Conn
	addr string
}

func (u *upstream) send(cmd cmd) error {
	p := &packet{Cmd: cmd}
	return u.sendpacket(p)
}

func (u *upstream) sendpacket(p *packet) error {
	switch u.proto {
	case tcp:
		err := u.encoder.Encode(p)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
				"cmd":   p.Cmd,
				"proto": u.proto,
			}).Warnln("send upstream cmd error")
		}
		return err
	case udp:
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(p); err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
				"cmd":   p.Cmd,
				"proto": u.proto,
			}).Warnln("send upstream cmd error")
		}
		var err error
		if u.udpaddr != nil { // server
			_, err = u.udpconn.WriteToUDP(buf.Bytes(), u.udpaddr)
		} else if u.conn != nil { // dialer
			_, err = u.conn.Write(buf.Bytes())
		} else {
			logrus.WithFields(logrus.Fields{
				"upstream": u,
			}).Warnln("upstream is not there")
		}
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
				"cmd":   p.Cmd,
				"proto": u,
			}).Warnln("send upstream error")
		}
		return err
	}
	return errors.New("send to unknown upstream protocol")
}

func (u *upstream) close() {
	if u.conn != nil {
		u.conn.Close()
		u.conn = nil
	}
	if u.udpconn != nil {
		u.udpconn.Close()
		u.udpconn = nil
	}
}

func (u *upstream) isAlive() bool {
	return keepalive > time.Duration(time.Now().UnixNano()-atomic.LoadInt64(&u.alive))
}

type streampool struct {
	*sync.Cond
	pool     []*upstream
	atomicid uint64
}

func newStreamPool() *streampool {
	return &streampool{
		// TODO: use RWMutex maybe?
		Cond: sync.NewCond(&sync.Mutex{}),
	}
}

func (pool *streampool) append(u *upstream) {
	pool.L.Lock()
	defer func() {
		pool.L.Unlock()
		pool.Broadcast()
	}()

	if u.uuid != 0 {
		for _, v := range pool.pool {
			if v.uuid == u.uuid {
				return
			}
		}
	}
	u.uuid = atomic.AddUint64(&pool.atomicid, 1)
	pool.pool = append(pool.pool, u)
}

func (pool *streampool) pickupstreams() []*upstream {
	pool.waitforalive()

	// TODO: pick udp and tcp equally
	pool.L.Lock()
	defer pool.L.Unlock()
	var alived []*upstream
	for _, v := range pool.pool {
		if v.isAlive() {
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
	alive := 0
	for _, v := range pool.pool {
		if v.isAlive() {
			alive++
		}
	}
	if alive >= 2 || alive >= len(pool.pool) {
		return true
	}
	return false
}

func (pool *streampool) waitforalive() {
	pool.L.Lock()
	for !pool.alive() {
		pool.Wait()
	}

	pool.L.Unlock()
}

func (pool *streampool) remove(u *upstream) {
	pool.L.Lock()
	for k, v := range pool.pool {
		if v.uuid == u.uuid {
			pool.pool = append(pool.pool[:k], pool.pool[k+1:]...)
		}
	}
	pool.L.Unlock()
	pool.Broadcast()
}
