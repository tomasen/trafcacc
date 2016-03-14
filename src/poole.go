// poolc.go 连接池为 backend 存放来自 frontend 之间的连接

package trafcacc

import (
	"encoding/gob"
	"sync"
)

// poole holds connections from frontend for backend
type poole struct {
	cond *sync.Cond
	pl   []*gob.Encoder
	id   int
}

func newPoole() *poole {
	return &poole{cond: sync.NewCond(&sync.Mutex{})}
}

func (p *poole) add(c *gob.Encoder) {
	p.cond.L.Lock()
	p.pl = append(p.pl, c)
	p.cond.L.Unlock()
	p.cond.Broadcast()
}

func (p *poole) next() *gob.Encoder {
	p.cond.L.Lock()
	for len(p.pl) == 0 {
		log.WithFields(log.Fields{
			"poolen": len(p.pl),
		}).Debugln("wait for connections from frontend")
		p.cond.Wait()
	}
	defer p.cond.L.Unlock()
	p.id++
	return p.pl[p.id%len(p.pl)]
}

func (p *poole) remove(c *gob.Encoder) {
	// TODO: need to be called some time
	p.cond.L.Lock()
	for id, v := range p.pl {
		if v == c {
			p.pl = append(p.pl[:id], p.pl[id+1:]...)
			return
		}
	}
	p.cond.L.Unlock()
	p.cond.Broadcast()
}
