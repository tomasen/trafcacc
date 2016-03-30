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
	return &node{
		pqs:  newPacketQueue(),
		pool: newStreamPool(),
		name: name,
	}
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
		n.pool.Broadcast()
	case ack:
		n.pool.cache.ack(p.Senderid, p.Connid, p.Seqid)
	case rqu:
		rp := n.pool.cache.get(p.Senderid, p.Connid, p.Seqid)
		if rp != nil {
			n.mux.Lock()
			now := time.Now().UnixNano()
			if atomic.LoadInt64(&rp.Time) < now-int64(rqudelay) {
				atomic.StoreInt64(&rp.Time, now)
				n.write(rp)
				// logrus.WithFields(logrus.Fields{
				// 	"Senderid": p.Senderid,
				// 	"Connid":   p.Connid,
				// 	"Seqid":    p.Seqid,
				// }).Debugln("response to packet request")
			}
			n.mux.Unlock()
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
		waiting := n.pqs.add(p)
		if waiting >= p.Seqid {
			if waiting != ^uint32(0) {
				n.mux.Lock()
				now := time.Now().UnixNano()
				if now > n.lastack {
					n.write(&packet{
						Senderid: p.Senderid,
						Connid:   p.Connid,
						Seqid:    p.Seqid,
						Cmd:      ack,
						Time:     now,
					})
					n.lastack = now + int64(time.Second)
				}
				n.mux.Unlock()
			}
			break
		}
		time.Sleep(rqudelay)
		stillwaiting := n.pqs.waiting(p.Senderid, p.Connid)
		if stillwaiting >= p.Seqid || stillwaiting != waiting {
			break
		}
		n.mux.Lock()
		now := time.Now().UnixNano()
		if now > n.lastrqu {
			n.write(&packet{
				Senderid: p.Senderid,
				Connid:   p.Connid,
				Seqid:    stillwaiting,
				Cmd:      rqu,
				Time:     time.Now().UnixNano(),
			})
			if logrus.GetLevel() >= logrus.DebugLevel {
				logrus.WithFields(logrus.Fields{
					"Connid":       p.Connid,
					"Seqid":        p.Seqid,
					"Waiting":      waiting,
					"StillWaiting": stillwaiting,
					"role":         n.role(),
				}).Debugln("send packet request")
			}
			n.lastrqu = now + int64(time.Second)
		}
		n.mux.Unlock()
	default:
		logrus.WithFields(logrus.Fields{
			"Cmd": p.Cmd,
		}).Warnln("unexpected Cmd in packet")
	}
}
