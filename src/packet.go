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

func (t *trafcacc) sendRaw(p packet) {

	// use cpool here , conn by connid
	conn := t.cpool.get(p.connid)
	u := t.upool.next()

	if conn == nil {
		// dial
		switch u.proto {
		case "tcp":
			conn, err := net.Dial("tcp", u.addr)
			if err != nil {
				// reply error
				t.replyPkt(packet{connid: p.connid})
				return
			}
			t.cpool.add(p.connid, conn)
			go func() {
				seqid := uint32(1)
				b := make([]byte, buffersize)
				for {
					n, err := conn.Read(b)
					if err != nil {
						break
					}
					t.replyPkt(packet{p.connid, seqid, b[0:n]})
					seqid++
				}
			}()
		}
	}

	if !t.cpool.dupChk(p.connid, p.seqid) {
		u.conn.Write(p.buf)
	}
}

// send packed data to backend
func (t *trafcacc) sendpkt(p packet) {
	u := t.upool.next()

	if u.conn == nil {
		// dial
		switch u.proto {
		case "tcp":
			conn, err := net.Dial("tcp", u.addr)
			if err != nil {
				// reply error
				t.replyRaw(packet{connid: p.connid})
				return
			}
			u.conn = conn
			u.encoder = gob.NewEncoder(conn)
			u.decoder = gob.NewDecoder(conn)
			// build reading slaves
			go func() {
				for {
					p := packet{}
					err := u.decoder.Decode(&p)
					if err != nil {
						break
					}
					t.replyRaw(p)
				}
				u.conn.Close()
				u.conn = nil
			}()
		case "udp":
			// TODO: udp
		}
	}

	err := u.encoder.Encode(&p)
	if err != nil {
		u.conn.Close()
		u.conn = nil
		// reply error
		t.replyRaw(packet{connid: p.connid})
		return
	}
}

func (t *trafcacc) replyRaw(p packet) {
	conn := t.cpool.get(p.connid)
	if conn == nil {
		log.Println("reply to no-exist client conn")
		return
	}
	if p.buf == nil {
		conn.Close()
		t.cpool.del(p.connid)
	} else {
		// get ride of duplicated connid+seqid
		// TODO: wait in case seqid is out of order
		if !t.cpool.dupChk(p.connid, p.seqid) {
			conn.Write(p.buf)
		}
	}
}

func (t *trafcacc) replyPkt(p packet) {
	conn := t.epool.next()
	conn.Encode(p)
}
