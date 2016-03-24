package trafcacc

import (
	"encoding/binary"
	"errors"
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

func (p *packet) copy() *packet {
	var buf []byte
	if len(p.Buf) > 0 {
		buf = make([]byte, len(p.Buf))
		copy(buf, p.Buf)
	}
	return &packet{
		Senderid: p.Senderid,
		Connid:   p.Connid,
		Seqid:    p.Seqid,
		Cmd:      p.Cmd,
		Buf:      buf,
	}
}

func (p *packet) encode(b []byte) (n int) {
	defer func() {
		if r := recover(); r != nil {
			n = -1
		}
	}()
	n = binary.PutUvarint(b, uint64(p.Senderid))
	n += binary.PutUvarint(b[n:], uint64(p.Connid))
	n += binary.PutUvarint(b[n:], uint64(p.Seqid))
	n += binary.PutUvarint(b[n:], uint64(p.Cmd))
	n += binary.PutUvarint(b[n:], uint64(len(p.Buf)))
	if len(p.Buf) > 0 {
		n += copy(b[n:], p.Buf)
		//b = append(b[:n], p.Buf...)
	}
	return n
}

func decodePacket(b []byte, p *packet) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("packet decode panic")
		}
	}()

	i, n := binary.Uvarint(b)
	p.Senderid = uint32(i)
	i, m := binary.Uvarint(b[n:])
	n += m
	p.Connid = uint32(i)

	i, m = binary.Uvarint(b[n:])
	n += m
	p.Seqid = uint32(i)

	i, m = binary.Uvarint(b[n:])
	n += m
	p.Cmd = cmd(i)

	i, m = binary.Uvarint(b[n:])
	if i > 0 {
		n += m
		p.Buf = b[n : n+int(i)]
	}

	return
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
		return true
	}

	if _, exist := q.queue[q.waitingSeqid]; exist {
		return true
	}

	return false
}

type packetQueue struct {
	queues map[uint64]*queue
	mux    sync.RWMutex
}

func newPacketQueue() *packetQueue {
	return &packetQueue{
		queues: make(map[uint64]*queue),
	}
}

func (pq *packetQueue) key(senderid, connid uint32) uint64 {
	return uint64(senderid)<<32 | uint64(connid)
}

func (pq *packetQueue) create(senderid, connid uint32) (isnew bool) {
	key := pq.key(senderid, connid)

	pq.mux.Lock()
	defer pq.mux.Unlock()
	if _, exist := pq.queues[key]; !exist {
		pq.queues[key] = newQueue()
		return true
	}
	return false
}

func (pq *packetQueue) close(senderid, connid uint32) {
	key := pq.key(senderid, connid)
	// TODO: wait queue drained cleanedup?
	pq.mux.Lock()
	q, exist := pq.queues[key]
	pq.mux.Unlock()
	if exist && q != nil {
		// set q.queue = nil ?
		atomic.StoreInt64(&q.closed, 1)
		q.Broadcast()

		go func() {
			// expired after 30 minutes
			<-time.After(30 * time.Minute)

			pq.mux.Lock()

			delete(pq.queues, key)
			pq.mux.Unlock()
		}()
	}
}

func (pq *packetQueue) add(p *packet) {
	key := pq.key(p.Senderid, p.Connid)

	pq.mux.Lock()
	q, exist := pq.queues[key]
	pq.mux.Unlock()

	if exist && q != nil {
		q.L.Lock()
		_, ok := q.queue[p.Seqid]
		if p.Seqid >= q.waitingSeqid && !ok {
			q.queue[p.Seqid] = p
			defer q.Broadcast()

		}
		q.L.Unlock()
	} else {
		logrus.WithFields(logrus.Fields{
			"packet": p,
			"exist":  exist,
			"q":      q,
			"key":    key,
			"pq":     pq,
		}).Warnln("packetQueue havn't been created, dropping")
	}
}

func (pq *packetQueue) waitforArrived(senderid, connid uint32) {
	key := pq.key(senderid, connid)

	pq.mux.Lock()
	q, exist := pq.queues[key]
	pq.mux.Unlock()

	if exist && q != nil {
		q.L.Lock()
		for !q.arrived() {
			q.Wait()
		}
		q.L.Unlock()
	} else {
		logrus.Warnln("waitforArrived() wait on deteled/closed queue")
	}
}

func (pq *packetQueue) isClosed(senderid, connid uint32) bool {
	key := pq.key(senderid, connid)

	pq.mux.Lock()
	q, exist := pq.queues[key]
	pq.mux.Unlock()
	if exist && q != nil {
		return q.isClosed()
	}
	return true
}

func (pq *packetQueue) pop(senderid, connid uint32) *packet {
	key := pq.key(senderid, connid)

	pq.mux.Lock()
	q, exist := pq.queues[key]
	pq.mux.Unlock()

	if exist && q != nil {
		// TODO: closed?
		// if q.isClosed() {
		// 	return nil
		// }

		q.L.Lock()
		p, exist := q.queue[q.waitingSeqid]
		if exist {
			delete(q.queue, q.waitingSeqid)
			q.waitingSeqid++
			q.L.Unlock()
			defer q.Broadcast()
			if p.Cmd == close || p.Cmd == closed {
				pq.close(senderid, connid)
				return nil
			}
			return p
		}
		q.L.Unlock()
	}

	return nil
}
