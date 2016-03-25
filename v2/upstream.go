package trafcacc

import (
	"encoding/gob"
	"errors"
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

		udpbuf := make([]byte, buffersize)

		n := p.encode(udpbuf)
		if n < 0 {
			logrus.WithFields(logrus.Fields{
				"cmd":   p.Cmd,
				"proto": u.proto,
			}).Warnln("send upstream cmd error")
		}
		var err error
		if u.udpaddr != nil { // server
			_, err = u.udpconn.WriteToUDP(udpbuf[:n], u.udpaddr)
		} else if u.conn != nil { // dialer
			_, err = u.conn.Write(udpbuf[:n])
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
	mux      *sync.RWMutex
	pool     []*upstream
	atomicid uint64
	alive    bool
	wg       sync.WaitGroup

	// for pick up streams
	tcpool                 []*upstream
	udpool                 []*upstream
	alived                 []*upstream
	tcplen, udplen, alvlen int

	// write
	werr atomic.Value
	rn   uint32
}

func newStreamPool() *streampool {
	mux := &sync.RWMutex{}
	pl := &streampool{
		// use RWMutex
		Cond: sync.NewCond(mux),
		mux:  mux,
	}

	go pl.updateloop()
	return pl
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

	pool.tcpool = pool.ensureCap(pool.tcpool)
	pool.udpool = pool.ensureCap(pool.udpool)
	pool.alived = pool.ensureCap(pool.alived)
}

func (pool *streampool) ensureCap(pl []*upstream) []*upstream {
	if cap(pl) < len(pool.pool) {
		ups := make([]*upstream, cap(pl)+10)
		for k, v := range pl {
			ups[k] = v
		}
		return ups
	}
	return pl
}

func (pool *streampool) pickupstreams() []*upstream {
	pool.waitforalive()

	// pick udp and tcp equally
	pool.mux.RLock()
	defer pool.mux.RUnlock()

	// pick one of each

	rn := int(atomic.AddUint32(&pool.rn, 2) - 2)

	switch {
	case pool.tcplen > 0 && pool.udplen > 0:
		// pick one of each
		return []*upstream{
			pool.udpool[rn%pool.udplen],
			pool.tcpool[rn%pool.tcplen],
			pool.udpool[(rn+1)%pool.udplen],
			pool.tcpool[(rn+1)%pool.udplen],
		}
	case pool.tcplen == 0 || pool.udplen == 0:
		// pick 1-2 alived
		return []*upstream{
			pool.alived[rn%pool.alvlen],
			pool.alived[(rn+1)%pool.alvlen],
		}
	}
	logrus.Warnln("no upstream avalible for pick")
	return nil

}

func (pool *streampool) waitforalive() {
	pool.L.Lock()
	for !pool.alive {
		pool.Wait()
	}
	pool.L.Unlock()
}

func (pool *streampool) updateloop() {
	for {
		pool.L.Lock()
		for !pool.updatealive() {
			pool.Wait()
		}
		pool.L.Unlock()
		pool.Broadcast()
		if pool.alive {
			<-time.After(time.Second)
		}
	}
}

// check if there is any alive upstream
func (pool *streampool) updatealive() (updated bool) {
	var tcpidx, udpidx, aliveidx int
	for _, v := range pool.pool {
		if v.isAlive() {
			switch v.proto {
			case tcp:
				pool.tcpool[tcpidx] = v
				tcpidx++
			case udp:
				pool.udpool[udpidx] = v
				udpidx++
			}
			pool.alived[aliveidx] = v
			aliveidx++
		}
	}
	if aliveidx > 0 {
		if pool.alive != true {
			updated = true
			pool.alive = true
		}
	} else {
		if pool.alive != false {
			updated = true
			pool.alive = false
		}
	}
	pool.tcplen = tcpidx
	pool.udplen = udpidx
	pool.alvlen = aliveidx
	return
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

	if pool.werr.Load() != nil {
		logrus.WithFields(logrus.Fields{
			"error": "no successed write",
		}).Warnln("encode packet to upstream error")
		return pool.werr.Load().(error)
	}

	// pick upstream tunnel and send packet
	for _, u := range pool.pickupstreams() {
		go func(up *upstream) {

			err := up.sendpacket(p)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"error": err,
				}).Warnln("Dialer encode packet to upstream errror")
				pool.werr.Store(err)
			}
		}(u)
	}

	return nil
}
