// poolu.go 连接池为 frontend 存放发向 backend 之间的连接

package trafcacc

import (
	"math/rand"
	"sync"
)

// poole holds connections for frontend to backend
type poolu struct {
	mux sync.RWMutex
	pl  []*upstream
	id  uint32
	end uint32
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
	p.id += uint32(rand.Intn(int(p.end)))
	return p.pl[p.id%p.end]
}
