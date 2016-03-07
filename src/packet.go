package trafcacc

import (
	"encoding/gob"
	"log"
	"net"
)

type packet struct {
	connid uint32
	seqid  uint32
	buf    []byte
}

var (
	atomicid uint32
)

// MARK: upstream
func sendpkt(p packet) {
	u := upool.next()

	if u.conn == nil {
		// dial
		switch u.proto {
		case "tcp":
			conn, err := net.Dial("tcp", u.addr)
			if err != nil {
				// reply error
				reply(p.connid, nil)
				return
			}
			u.conn = conn
			u.encoder = gob.NewEncoder(conn)
			u.decoder = gob.NewDecoder(conn)
			// build reading slaves
			go func() {
				for {
					p0 := &packet{}
					err := dec.Decode(p)
					if err == nil {
						break
					}
					replyRaw(p.connid, p.buf)
				}
				u.conn.Close()
				u.conn = nil
			}()
		case "udp":
			// TODO
		}
	}

	err := encoder.Encode(&p)
	if err != nil {
		u.conn.Close()
		u.conn = nil
		// reply error
		replyRaw(p.connid, nil)
		return
	}
}

func replyRaw(connid uint32, buf []byte) {
	conn := cpool.get(connid)
	if conn == nil {
		log.Println("reply to no-exist client conn")
		return
	}
	if buf == nil {
		conn.Close()
		cpool.del(connid)
	} else {
		// TODO: get ride of duplicated connid+seqid
		conn.Write(buf)
	}
}
