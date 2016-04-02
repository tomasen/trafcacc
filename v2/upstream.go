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
	sent    uint64
	recv    uint64
	jitter  int64
	latency int64
	closed  int32

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

func newUpstream(proto string) *upstream {
	return &upstream{
		proto:  proto,
		jitter: int64(^uint64(0) >> 1),
	}
}

func (u *upstream) send(cmd cmd) error {
	p := &packet{
		Cmd:  cmd,
		Time: time.Now().UnixNano(),
	}
	return u.sendpacket(p)
}

func (u *upstream) sendpacket(p *packet) error {
	atomic.AddUint64(&u.sent, uint64(len(p.Buf)))

	switch u.proto {
	case tcp:
		p.lock.RLock()
		err := u.encoder.Encode(p)
		p.lock.RUnlock()
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
	atomic.StoreInt32(&u.closed, 1)
}

func (u *upstream) isAlive() bool {
	return atomic.LoadInt32(&u.closed) == 0 &&
		atomic.LoadInt64(&u.latency) < int64(time.Second) &&
		keepalive > time.Duration(time.Now().UnixNano()-atomic.LoadInt64(&u.alive))
}

type streampool struct {
	*sync.RWMutex
	pool     []*upstream
	atomicid uint64
	alive    int32
	wg       sync.WaitGroup

	// for pick up streams
	tcpool                 []*upstream
	udpool                 []*upstream
	alived                 []*upstream
	tcplen, udplen, alvlen int

	// write
	rn    uint32
	cache *writeCache
}

func newStreamPool() *streampool {
	pl := &streampool{
		// use RWMutex
		RWMutex: &sync.RWMutex{},
		cache:   newWriteCache(),
	}

	go pl.updateloop()
	return pl
}

func (pool *streampool) append(u *upstream, grp int) {
	pool.Lock()
	defer func() {
		pool.Unlock()
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

func (pool *streampool) pickupstreams(udp bool) []*upstream {
	pool.waitforalive()

	// pick udp and tcp equally
	pool.RLock()
	defer pool.RUnlock()

	// pick one of each

	switch {
	case udp && pool.udplen > 0:
		rn := int(atomic.AddUint32(&pool.rn, 1) - 1)
		return []*upstream{pool.udpool[rn%pool.udplen]}
	case pool.tcplen > 0 && pool.udplen > 0:
		// pick one of each
		rn := int(atomic.AddUint32(&pool.rn, 1) - 1)

		return []*upstream{
			pool.udpool[rn%pool.udplen],
			pool.tcpool[(rn+pool.tcplen/2)%pool.tcplen],
			// pool.udpool[(rn+1)%pool.udplen],
			// pool.tcpool[(rn+1)%pool.udplen],
		}
	case pool.tcplen == 0 || pool.udplen == 0:
		// pick 2 alived
		rn := int(atomic.AddUint32(&pool.rn, 2) - 2)

		return []*upstream{
			pool.alived[rn%pool.alvlen],
			pool.alived[(rn+1)%pool.alvlen],
		}
	}
	logrus.Warnln("no upstream avalible for pick")
	return nil
}

func (pool *streampool) waitforalive() {

	for {
		if atomic.LoadInt32(&pool.alive) != 0 {
			break
		}
		<-time.After(time.Millisecond * 200)
	}

}

func (pool *streampool) updateloop() {
	for {
		pool.updatealive()

		if atomic.LoadInt32(&pool.alive) != 0 {
			<-time.After(time.Second)
		} else {
			<-time.After(time.Millisecond * 200)
		}
	}
}

// check if there is any alive upstream
func (pool *streampool) updatealive() (updated bool) {
	pool.Lock()
	defer pool.Unlock()
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
		if atomic.LoadInt32(&pool.alive) == 0 {
			updated = true
			atomic.StoreInt32(&pool.alive, 1)
		}
	} else {
		if atomic.LoadInt32(&pool.alive) != 0 {
			updated = true
			atomic.StoreInt32(&pool.alive, 0)
		}
	}
	pool.tcplen = tcpidx
	pool.udplen = udpidx
	pool.alvlen = aliveidx
	return
}

func (pool *streampool) remove(u *upstream) {
	pool.Lock()
	for k, v := range pool.pool {
		if v.uuid == u.uuid {
			pool.pool = append(pool.pool[:k], pool.pool[k+1:]...)
		}
	}
	pool.Unlock()
}

func (pool *streampool) write(p *packet) {

	// pick upstream tunnel and send packet
	for _, u := range pool.pickupstreams(p.udp) {
		go func(up *upstream) {
			err := up.sendpacket(p)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"error": err,
				}).Warnln("encode packet to upstream errror")
				up.close()
			}
		}(u)
	}

	// put packet in cache
	if p.Cmd == data {
		pool.cache.add(p)
	}

	return
}
