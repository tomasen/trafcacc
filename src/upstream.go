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
	u.mux.Lock()
	defer u.mux.Unlock()
	if u.conn != nil {
		u.conn.Close()
		u.conn = nil
	}
}
