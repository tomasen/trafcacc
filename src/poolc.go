package trafcacc

import (
	"log"
	"net"
	"sync"
)

const maxtrackconn = uint32(^uint16(0))

// pool of connections
type poolc struct {
	mux     sync.RWMutex
	pool    map[uint32]net.Conn
	lastseq [maxtrackconn]uint32
}

func NewPoolc() *poolc {
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

func (p *poolc) dupChk(connid uint32, seqid uint32) bool {
	// TODO: mute xmaybe
	if seqid == p.lastseq[connid%maxtrackconn] {
		return true
	}
	p.lastseq[connid%maxtrackconn] = seqid
	return false
}
