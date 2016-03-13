// poolc.go 连接池为 backend 存放来自 frontend 之间的连接

package trafcacc

import (
	"encoding/gob"
	"sync"
)

// poole holds connections from frontend for backend
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
	if len(p.pl) == 0 {
		return nil
	}
	p.id++
	return p.pl[p.id%len(p.pl)]
}

func (p *poole) remove(c *gob.Encoder) {
	// TODO: need to be called some time
	p.mux.Lock()
	defer p.mux.Unlock()
	for id, v := range p.pl {
		if v == c {
			p.pl = append(p.pl[:id], p.pl[id+1:]...)
			return
		}
	}
}
