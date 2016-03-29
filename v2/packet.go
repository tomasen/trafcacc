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
	rqu
	ack
)

type packet struct {
	Senderid uint32
	Connid   uint32
	Seqid    uint32
	Buf      []byte
	Cmd      cmd
	udp      bool
	Time     int64
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
		udp:      p.udp,
		Time:     p.Time,
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
	n += binary.PutUvarint(b[n:], uint64(atomic.LoadInt64(&p.Time)))
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

	err = errors.New("packet decode err")
	i, n := binary.Uvarint(b)
	if n <= 0 {
		return
	}
	p.Senderid = uint32(i)
	i, m := binary.Uvarint(b[n:])
	if m <= 0 {
		return
	}
	n += m
	p.Connid = uint32(i)

	i, m = binary.Uvarint(b[n:])
	if m <= 0 {
		return
	}
	n += m
	p.Seqid = uint32(i)

	i, m = binary.Uvarint(b[n:])
	if m <= 0 {
		return
	}
	n += m
	p.Cmd = cmd(i)

	i, m = binary.Uvarint(b[n:])
	if m <= 0 {
		return
	}
	n += m
	p.Time = int64(i)

	i, m = binary.Uvarint(b[n:])
	if m <= 0 {
		return
	}
	if i > 0 {
		n += m
		p.Buf = b[n : n+int(i)]
	}

	return nil
}

type queue struct {
	*sync.Cond
	queue        map[uint32]*packet
	nxrqutime    map[uint32]time.Time
	waitingSeqid uint32
	closed       int64
}

func newQueue() *queue {
	return &queue{
		Cond:         sync.NewCond(&sync.Mutex{}),
		queue:        make(map[uint32]*packet),
		nxrqutime:    make(map[uint32]time.Time),
		waitingSeqid: 1,
	}
}

func (q *queue) len() int {
	return len(q.queue)
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
	popudp uint64
	poptcp uint64
}

func newPacketQueue() *packetQueue {
	return &packetQueue{
		queues: make(map[uint64]*queue),
	}
}

func (pq *packetQueue) create(senderid, connid uint32) (isnew bool) {
	key := packetKey(senderid, connid)

	pq.mux.Lock()
	defer pq.mux.Unlock()
	if _, exist := pq.queues[key]; !exist {
		pq.queues[key] = newQueue()
		return true
	}
	return false
}

func (pq *packetQueue) close(senderid, connid uint32) {
	key := packetKey(senderid, connid)
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

func (pq *packetQueue) len() (n int) {
	pq.mux.RLock()
	defer pq.mux.RUnlock()
	for _, v := range pq.queues {
		v.L.Lock()
		n += len(v.queue)
		v.L.Unlock()
	}
	return
}

func (pq *packetQueue) add(p *packet) (waitingSeqid uint32) {
	key := packetKey(p.Senderid, p.Connid)

	pq.mux.Lock()
	q, exist := pq.queues[key]
	pq.mux.Unlock()

	waitingSeqid = ^uint32(0) // wait no more

	if exist && q != nil {
		q.L.Lock()
		_, ok := q.queue[p.Seqid]
		if p.Seqid >= q.waitingSeqid && !ok {
			q.queue[p.Seqid] = p
			defer q.Broadcast()

			if p.Seqid >= q.waitingSeqid {
				if t, ok := q.nxrqutime[q.waitingSeqid]; !ok || time.Now().After(t) {
					waitingSeqid = q.waitingSeqid
				}
			}
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
	return
}

func (pq *packetQueue) waiting(senderid, connid uint32) (waitingSeqid uint32) {
	key := packetKey(senderid, connid)

	pq.mux.Lock()
	q, exist := pq.queues[key]
	pq.mux.Unlock()

	if exist && q != nil {
		q.L.Lock()
		if t, ok := q.nxrqutime[q.waitingSeqid]; !ok || time.Now().After(t) {
			waitingSeqid = q.waitingSeqid
			q.nxrqutime[q.waitingSeqid] = time.Now().Add(rqudelay)
		}
		q.L.Unlock()
		return
	}
	return ^uint32(0)
}

func (pq *packetQueue) waitforArrived(senderid, connid uint32) {
	key := packetKey(senderid, connid)

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
	key := packetKey(senderid, connid)

	pq.mux.Lock()
	q, exist := pq.queues[key]
	pq.mux.Unlock()
	if exist && q != nil {
		return q.isClosed()
	}
	return true
}

func (pq *packetQueue) pop(senderid, connid uint32) *packet {
	key := packetKey(senderid, connid)

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
			if _, ok := q.queue[q.waitingSeqid]; ok {
				delete(q.queue, q.waitingSeqid)
			}
			q.waitingSeqid++
			q.L.Unlock()
			defer q.Broadcast()
			if p.Cmd == close || p.Cmd == closed {
				pq.close(senderid, connid)
				return nil
			}
			if p.udp {
				atomic.AddUint64(&pq.popudp, uint64(len(p.Buf)))
			} else {
				atomic.AddUint64(&pq.poptcp, uint64(len(p.Buf)))
			}
			return p
		}
		q.L.Unlock()
	}

	return nil
}
