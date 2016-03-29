package trafcacc

import (
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
)

type node struct {
	pool *streampool
	pqs  *packetQueue
	name string
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

func (n *node) write(p *packet) error {
	return n.pool.write(p)
}

func (n *node) proc(u *upstream, p *packet) {

	atomic.AddUint64(&u.recv, uint64(len(p.Buf)))

	switch p.Cmd {
	case ping, pong:
		atomic.StoreInt64(&u.alive, time.Now().UnixNano())
		n.pool.Broadcast()
	case ack:
		n.pool.cache.ack(p.Senderid, p.Connid, p.Seqid)
	case rqu:
		rp := n.pool.cache.get(p.Senderid, p.Connid, p.Seqid)
		if rp != nil {
			n.write(rp)
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
			break
		}
		time.Sleep(rqudelay)
		stillwaiting := n.pqs.waiting(p.Senderid, p.Connid)
		if stillwaiting >= p.Seqid || stillwaiting != waiting {
			break
		}
		n.write(&packet{
			Senderid: p.Senderid,
			Connid:   p.Connid,
			Seqid:    stillwaiting,
			Cmd:      rqu,
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

	default:
		logrus.WithFields(logrus.Fields{
			"Cmd": p.Cmd,
		}).Warnln("unexpected Cmd in packet")
	}
}
