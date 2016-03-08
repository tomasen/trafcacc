package trafcacc

import (
	"log"
	"net"
	"sync"
)

// TODO: track all connections
const maxtrackconn = uint32(^uint16(0))

// pool of connections
type poolc struct {
	mux     sync.RWMutex
	pool    map[uint32]net.Conn
	lastseq [maxtrackconn]uint32
	sedcond [maxtrackconn]*sync.Cond
	ta      *trafcacc
}

func newPoolc() *poolc {
	return &poolc{pool: make(map[uint32]net.Conn)}
}

func (p *poolc) add(id uint32, conn net.Conn) {
	log.Println("poolc add")
	p.mux.Lock()
	defer p.mux.Unlock()
	log.Println("poolc add2")
	p.pool[id] = conn
	log.Println("poolc add3")
}

func (p *poolc) get(id uint32) net.Conn {
	p.mux.RLock()
	defer p.mux.RUnlock()
	return p.pool[id]
}

func (p *poolc) del(id uint32) {
	p.mux.Lock()
	defer p.mux.Unlock()
	delete(p.pool, id)
}

// TODO: ensure Write in sequence
func (p *poolc) ensure(pkt packet, conn net.Conn) {
	// TODO: mutex?
	idx := pkt.Connid % maxtrackconn
	lastseq := p.lastseq[idx]

	cond := p.sedcond[idx]
	if cond == nil {
		cond = sync.NewCond(&sync.Mutex{})
		p.sedcond[idx] = cond
		log.Println(p.ta.isbackend, "new cond", idx)
	}
	defer func() {
		cond.L.Lock()
		p.lastseq[idx] = pkt.Seqid
		cond.L.Unlock()
		cond.Broadcast()
		log.Println(p.ta.isbackend, "broadcast", idx, pkt.Seqid, cond)
	}()

	if pkt.Buf == nil {
		// TODO: close connection?
		return
	}

	log.Println(p.ta.isbackend, "ensure", idx, pkt.Connid, pkt.Seqid, lastseq)
	switch {
	case pkt.Seqid <= lastseq:
		// get ride of duplicated connid+seqid
		return
	case pkt.Seqid == lastseq+1:
		conn.Write(pkt.Buf)
		return
	default:
		// wait if case seqid is out of order
		cond.L.Lock()
		for p.lastseq[idx]+1 != pkt.Seqid {
			if pkt.Seqid <= p.lastseq[idx] {
				// get ride of duplicated connid+seqid
				log.Println("get ride2", pkt.Seqid, p.lastseq[idx])
				cond.L.Unlock()
				return
			}
			log.Println(p.ta.isbackend, "wait seq1", idx, pkt.Connid, pkt.Seqid, p.lastseq[idx], cond)
			cond.Wait()
			log.Println(p.ta.isbackend, "wait seq2", p.lastseq[idx], pkt.Seqid)
		}
		cond.L.Unlock()
		conn.Write(pkt.Buf)
		return
	}
}
