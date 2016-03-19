package trafcacc

import (
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
)

type cmd uint8

const (
	data cmd = iota
	close
	closed
	connect
	connected
	ping
	pong
)

type packet struct {
	Senderid uint32
	Connid   uint32
	Seqid    uint32
	Buf      []byte
	Cmd      cmd
}

type queue struct {
	queue        map[uint32]*packet
	cond         *sync.Cond
	waitingSeqid uint32
}

type packetQueue struct {
	queue  map[uint32]map[uint32]*queue
	closed map[uint32]map[uint32]struct{}
}

func newPacketQueue() *packetQueue {
	return &packetQueue{
		queue:  make(map[uint32]map[uint32]*queue),
		closed: make(map[uint32]map[uint32]struct{}),
	}
}

func (pq *packetQueue) create(senderid, connid uint32) (isnew bool) {

	if _, ok := pq.queue[senderid]; !ok {
		pq.queue[senderid] = make(map[uint32]*queue)
	}

	if _, ok := pq.queue[senderid][connid]; !ok {
		delete(pq.closed[senderid], connid)

		pq.queue[senderid][connid] = &queue{
			queue:        make(map[uint32]*packet),
			cond:         sync.NewCond(&sync.Mutex{}),
			waitingSeqid: 1,
		}
		return true
	}
	return false
}

func (pq *packetQueue) close(senderid, connid uint32) {
	// TODO: wait queue drained cleanedup
	if q, ok := pq.queue[senderid][connid]; ok {
		delete(pq.queue[senderid], connid)
		pq.closed[senderid][connid] = struct{}{}

		q.cond.Broadcast()

		go func() {
			// expired after 30 minutes
			<-time.After(30 * time.Minute)

			if _, ok := pq.closed[senderid][connid]; ok {
				delete(pq.closed[senderid], connid)
			}
		}()
	}
}

func (pq *packetQueue) add(p *packet) {
	if _, ok := pq.closed[p.Senderid][p.Connid]; !ok {
		if q, ok := pq.queue[p.Senderid][p.Connid]; ok {
			// TODO: lock
			if _, ok := q.queue[p.Seqid]; !ok {
				q.cond.L.Lock()
				q.queue[p.Seqid] = p
				q.cond.L.Unlock()
				q.cond.Broadcast()
			} // else drop duplicated packet
		} else {
			logrus.WithFields(logrus.Fields{
				"packet": p,
			}).Fatalln("packetQueue havn't been created")
		}
	} // else drop closed packet
}

func (pq *packetQueue) arrived(senderid, connid uint32) bool {
	if pq.isclosed(senderid, connid) {
		return true // if connection closed
	}
	if q, ok := pq.queue[senderid][connid]; ok {
		if _, ok := q.queue[q.waitingSeqid]; ok {
			return true
		}
	}
	return false
}

func (pq *packetQueue) isclosed(senderid, connid uint32) bool {
	if _, ok := pq.closed[senderid][connid]; ok {
		return true // connection closed
	}
	return false
}

func (pq *packetQueue) pop(senderid, connid uint32) *packet {
	if q, ok := pq.queue[senderid][connid]; ok {
		if p, ok := q.queue[q.waitingSeqid]; ok {
			delete(q.queue, q.waitingSeqid)
			q.waitingSeqid++
			return p
		}
	}
	return nil
}

func (pq *packetQueue) cond(senderid, connid uint32) *sync.Cond {
	if q, ok := pq.queue[senderid][connid]; ok {
		return q.cond
	}
	return nil
}
