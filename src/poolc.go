package trafcacc

import (
	log "github.com/Sirupsen/logrus"
	"net"
	"sync"
)

// pool of connections
type poolc struct {
	mux  sync.RWMutex
	pool map[uint32]net.Conn
}

func newPoolc() *poolc {
	return &poolc{pool: make(map[uint32]net.Conn)}
}

func (p *poolc) add(id uint32, conn net.Conn) {
	p.mux.Lock()
	defer p.mux.Unlock()
	log.Debugln("poolc add")
	p.pool[id] = conn
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
