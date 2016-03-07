package trafcacc

import (
	"encoding/gob"
	"sync"
)

// pool of upstreams
type poolu struct {
	pl  []*upstream
	id  int
	end int
}

func (p *poolu) append(u *upstream) {
	p.pl = append(p.pl, u)
	p.end++
}

func (p *poolu) next() *upstream {
	p.id++
	return p.pl[p.id%p.end]
}

type poole struct {
	mux sync.RWMutex
	pl  []*gob.Encoder
	id  int
	end int
}

func (p *poole) add(c *gob.Encoder) int {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.pl = append(p.pl, c)
	r := p.end
	p.end++
	return r
}

func (p *poole) next() *gob.Encoder {
	p.mux.RLock()
	defer p.mux.RUnlock()
	p.id++
	return p.pl[p.id%p.end]
}

func (p *poole) remove(id int) {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.pl = append(p.pl[:id], p.pl[id+1:]...)
}
