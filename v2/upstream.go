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
	grp   int

	// status recorder
	sent uint64
	recv uint64

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
	atomic.AddUint64(&u.sent, uint64(len(p.Buf)))
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

func (pool *streampool) append(u *upstream, grp int) {
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
	u.grp = grp
	pool.pool = append(pool.pool, u)
}

func (pool *streampool) pickupstreams() []*upstream {
	pool.waitforalive()

	// pick udp and tcp equally
	pool.L.Lock()
	defer pool.L.Unlock()
	var tcpalived []*upstream
	var udpalived []*upstream
	for _, v := range pool.pool {
		if v.isAlive() {
			switch v.proto {
			case tcp:
				tcpalived = append(tcpalived, v)
			case udp:
				udpalived = append(udpalived, v)
			}
		}
	}
	var alived []*upstream
	// avoid duplicate
	udpArraySize := len(udpalived)
	tcpArraySize := len(tcpalived)

	if udpArraySize > 0 {
		idx := rand.Intn(udpArraySize)
		alived = append(alived, udpalived[idx])
		if tcpArraySize == 0 {
			return append(alived, udpalived[(idx+1)%udpArraySize])
		}
	}

	switch tcpArraySize {
	case 1:
		alived = append(alived, tcpalived[0])
	default:
		if len(alived) == 0 {
			idx := rand.Intn(tcpArraySize)
			return []*upstream{tcpalived[idx], tcpalived[(idx+1)%tcpArraySize]}
		}
		return append(alived, tcpalived[rand.Intn(tcpArraySize)])
	}
	return nil
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

func (pool *streampool) write(p *packet) error {
	var successed uint32

	var wg sync.WaitGroup
	// pick upstream tunnel and send packet
	for _, u := range pool.pickupstreams() {
		wg.Add(1)
		go func(up *upstream) {
			defer wg.Done()
			err := up.sendpacket(p)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"error": err,
				}).Warnln("Dialer encode packet to upstream errror")
			} else {
				atomic.StoreUint32(&successed, 1)
			}
		}(u)
	}
	wg.Wait()
	if successed != 0 {
		return nil
	}

	// return error if all failed
	return errors.New("dialer encoder error")
}
