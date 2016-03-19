package trafcacc

import (
	"sync"
	"sync/atomic"
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
	*sync.Cond
	queue        map[uint32]*packet
	waitingSeqid uint32
	closed       int64
}

func newQueue() *queue {
	return &queue{
		Cond:         sync.NewCond(&sync.Mutex{}),
		queue:        make(map[uint32]*packet),
		waitingSeqid: 1,
	}
}

func (q *queue) isClosed() bool {
	return atomic.LoadInt64(&q.closed) != 0
}

func (q *queue) arrived() bool {
	if q.isClosed() {
		// TODO: ?? how to flush
		return true
	}

	if _, exist := q.queue[q.waitingSeqid]; exist {
		return true
	}

	return false
}

type packetQueue struct {
	queue map[uint64]*queue
	mux   sync.RWMutex
}

func newPacketQueue() *packetQueue {
	return &packetQueue{
		queue: make(map[uint64]*queue),
	}
}

func (pq *packetQueue) key(senderid, connid uint32) uint64 {
	return uint64(senderid)<<32 | uint64(connid)
}

func (pq *packetQueue) create(senderid, connid uint32) (isnew bool) {
	key := pq.key(senderid, connid)

	pq.mux.Lock()
	defer pq.mux.Unlock()
	if _, exist := pq.queue[key]; !exist {
		pq.queue[key] = newQueue()
		return true
	}
	return false
}

func (pq *packetQueue) close(senderid, connid uint32) {
	key := pq.key(senderid, connid)
	// TODO: wait queue drained cleanedup?
	pq.mux.Lock()
	q, exist := pq.queue[key]
	pq.mux.Unlock()
	if exist && q != nil {
		// set q.queue = nil ?
		atomic.StoreInt64(&q.closed, 1)
		q.Broadcast()

		go func() {
			// expired after 30 minutes
			<-time.After(30 * time.Minute)

			pq.mux.Lock()
			delete(pq.queue, key)
			pq.mux.Unlock()
		}()
	}
}

func (pq *packetQueue) add(p *packet) {
	key := pq.key(p.Senderid, p.Connid)

	pq.mux.Lock()
	q, exist := pq.queue[key]
	pq.mux.Unlock()

	if exist && q != nil {
		q.L.Lock()
		q.queue[p.Seqid] = p
		q.L.Unlock()
		q.Broadcast()
	} else {
		logrus.WithFields(logrus.Fields{
			"packet": p,
		}).Fatalln("packetQueue havn't been created")
	}
}

func (pq *packetQueue) waitforArrived(senderid, connid uint32) {
	key := pq.key(senderid, connid)

	pq.mux.Lock()
	q, exist := pq.queue[key]
	pq.mux.Unlock()

	if exist && q != nil {
		q.L.Lock()
		for !q.arrived() {
			q.Wait()
		}
		q.L.Unlock()
	} else {
		logrus.Fatalln("waitforArrived() wait on deteled queue")
	}
}

func (pq *packetQueue) isClosed(senderid, connid uint32) bool {
	key := pq.key(senderid, connid)

	pq.mux.Lock()
	q, exist := pq.queue[key]
	pq.mux.Unlock()
	if exist && q != nil {
		return q.isClosed()
	}
	return true
}

func (pq *packetQueue) pop(senderid, connid uint32) *packet {
	key := pq.key(senderid, connid)

	pq.mux.Lock()
	q, exist := pq.queue[key]
	pq.mux.Unlock()

	if exist && q != nil {
		q.L.Lock()
		defer q.L.Unlock()
		if p, exist := q.queue[q.waitingSeqid]; exist {
			delete(q.queue, q.waitingSeqid)
			q.waitingSeqid++
			q.Broadcast()
			return p
		}
	}

	return nil
}
