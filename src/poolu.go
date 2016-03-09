package trafcacc

import (
	"encoding/gob"
	"sync"
	"sync/atomic"
)

// pool of upstreams
type poolu struct {
	mux sync.RWMutex
	pl  []*upstream
	id  int32
	end int32
}

func (p *poolu) append(u *upstream) {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.pl = append(p.pl, u)
	p.end++
}

func (p *poolu) next() *upstream {
	p.mux.RLock()
	defer p.mux.RUnlock()
	id := atomic.AddInt32(&p.id, 1)
	return p.pl[id%p.end]
}

type poole struct {
	mux sync.Mutex
	pl  []*gob.Encoder
	id  int
}

func (p *poole) add(c *gob.Encoder) {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.pl = append(p.pl, c)
}

func (p *poole) next() *gob.Encoder {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.id++
	return p.pl[p.id%len(p.pl)]
}

func (p *poole) remove(c *gob.Encoder) {
	p.mux.Lock()
	defer p.mux.Unlock()
	for id, v := range p.pl {
		if v == c {
			p.pl = append(p.pl[:id], p.pl[id+1:]...)
		}
	}
}
