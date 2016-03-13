// poolu.go 连接池为 frontend 存放发向 backend 之间的连接

package trafcacc

import (
	"sync"
	"sync/atomic"
)

// poole holds connections for frontend to backend
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
