// poolc.go 存放 backend 和 remote 目标服务之间的连接池

package trafcacc

import (
	"net"
	"sync"

	log "github.com/Sirupsen/logrus"
)

// poolc holds connections for backend to remote
type poolc struct {
	mux    sync.RWMutex
	pool   map[uint32]net.Conn
	closed map[uint32]struct{}
	lastid uint32
}

func newPoolc() *poolc {
	return &poolc{
		pool:   make(map[uint32]net.Conn),
		closed: make(map[uint32]struct{}),
		lastid: 1,
	}
}

// true if it is a connection that already closed
func (p *poolc) shouldDrop(id uint32) bool {
	p.mux.Lock()
	defer p.mux.Unlock()
	if id < p.lastid {
		return true
	}
	if _, ok := p.closed[id]; ok {
		return true
	}
	return false
}

func (p *poolc) add(id uint32, conn net.Conn) error {
	p.mux.Lock()
	defer p.mux.Unlock()
	log.WithFields(log.Fields{
		"connid": id,
	}).Debugln("poolc add")
	p.pool[id] = conn
	return nil
}

func (p *poolc) get(id uint32) net.Conn {
	p.mux.RLock()
	defer p.mux.RUnlock()
	return p.pool[id]
}

func (p *poolc) del(id uint32) {
	p.mux.Lock()
	defer p.mux.Unlock()
	log.WithFields(log.Fields{
		"connid": id,
	}).Debugln("poolc delete")
	delete(p.pool, id)
	p.closed[id] = struct{}{}
	if p.lastid < id {
		for i := p.lastid; i <= id; i++ {
			if _, ok := p.closed[i]; ok {
				p.lastid = i
				delete(p.closed, i)
			} else {
				break
			}
		}
	}
}
