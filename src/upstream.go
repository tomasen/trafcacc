// upstream.go 为 frontend 存放发向 backend 的连接的连接信息

package trafcacc

import (
	"encoding/gob"
	"net"
	"sync"
)

// upstream is connection manager for frontend to backend
type upstream struct {
	proto   string
	addr    string
	conn    net.Conn
	encoder *gob.Encoder
	decoder *gob.Decoder
	mux     sync.RWMutex
}

func (u *upstream) close() {
	// TODO: need to be closed some time
	u.mux.Lock()
	defer u.mux.Unlock()
	if u.conn != nil {
		u.conn.Close()
		u.conn = nil
	}
}
