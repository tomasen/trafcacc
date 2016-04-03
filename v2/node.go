package trafcacc

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
)

type node struct {
	pool    *streampool
	pqs     *packetQueue
	name    string
	lastack int64
	lastrqu int64
	mux     sync.Mutex
}

func newNode(name string) *node {
	n := &node{
		pqs:  newPacketQueue(),
		pool: newStreamPool(),
		name: name,
	}
	go n.rquloop()
	return n
}

func (n *node) streampool() *streampool {
	return n.pool
}

func (n *node) pq() *packetQueue {
	return n.pqs
}

func (n *node) role() string {
	return n.name
}

func (n *node) write(p *packet) {
	n.pool.write(p)
}

func (n *node) proc(u *upstream, p *packet) {

	atomic.AddUint64(&u.recv, uint64(len(p.Buf)))

	now := time.Now().UnixNano()
	if p.Time != 0 {
		jitter := now - p.Time
		jj := atomic.LoadInt64(&u.jitter)
		if jj > jitter {
			atomic.StoreInt64(&u.jitter, jitter)
		} else {
			atomic.StoreInt64(&u.latency, jitter-jj)
		}
	}

	switch p.Cmd {
	case ping, pong:
		atomic.StoreInt64(&u.alive, now)
	case ack:
		n.pool.cache.ack(p.Senderid, p.Connid, p.Seqid)
	case rqu:
		rp := n.pool.cache.get(p.Senderid, p.Connid, p.Seqid)
		if rp != nil {
			now := time.Now().UnixNano()
			rp.lock.Lock()
			if rp.Time < now-int64(rqudelay) {
				rp.Time = now
				//rp.udp = true
				rp.lock.Unlock()
				n.write(rp)
				logrus.WithFields(logrus.Fields{
					"Senderid": p.Senderid,
					"Connid":   p.Connid,
					"Seqid":    p.Seqid,
				}).Debugln("response to packet request")
			} else {
				rp.lock.Unlock()
			}
		} else {
			logrus.WithFields(logrus.Fields{
				"Senderid": p.Senderid,
				"Connid":   p.Connid,
				"Seqid":    p.Seqid,
			}).Warnln("unable to fullfile packet request")
		}
	}
	return
}

func (n *node) push(p *packet) {
	switch p.Cmd {
	case connected, connect:
		// TODO: maybe move d.pqs.create(p.Senderid, p.Connid) here?
	case closed, close:
		n.pqs.add(p)
		n.pool.cache.close(p.Senderid, p.Connid)
	case data: //data
		if n.pqs.add(p) >= p.Seqid {
			n.mux.Lock()
			now := time.Now().UnixNano()
			if now > n.lastack {
				n.write(&packet{
					Senderid: p.Senderid,
					Connid:   p.Connid,
					Seqid:    p.Seqid,
					Cmd:      ack,
					udp:      true,
					Time:     now,
				})
				n.lastack = now + int64(time.Second)
			}
			n.mux.Unlock()
		}
	default:
		logrus.WithFields(logrus.Fields{
			"Cmd": p.Cmd,
		}).Warnln("unexpected Cmd in packet")
	}
}

func (n *node) rquloop() {
	for {
		time.Sleep(rqudelay)
		now := time.Now()
		n.pqs.mux.RLock()
		for k, v := range n.pqs.queues {
			v.L.Lock()
			_, exist := v.queue[v.waitingSeqid]
			if v.maxseqid > v.waitingSeqid && !exist && v.waitTime.Before(now.Add(-rqudelay)) {
				senderid, connid := unpacketKey(k)
				waiting := v.waitingSeqid
				v.waitTime = now.Add(rqudelay)
				go func() {
					n.write(&packet{
						Senderid: senderid,
						Connid:   connid,
						Seqid:    waiting,
						Cmd:      rqu,
						Time:     now.UnixNano(),
					})
					if logrus.GetLevel() >= logrus.DebugLevel {
						logrus.WithFields(logrus.Fields{
							"Connid":       connid,
							"StillWaiting": waiting,
							"role":         n.role(),
						}).Debugln("send packet request")
					}
				}()
			}
			v.L.Unlock()
		}
		n.pqs.mux.RUnlock()
	}
}
