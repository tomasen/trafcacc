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

func sendRaw(p packet) {

	// use cpool here , conn by connid
	conn := cpool.get(p.connid)
	u := upool.next()

	if conn == nil {
		// dial
		switch u.proto {
		case "tcp":
			conn, err := net.Dial("tcp", u.addr)
			if err != nil {
				// reply error
				replyPkt(packet{connid: p.connid})
				return
			}
			cpool.add(p.connid, conn)
			go func() {
				seqid := uint32(1)
				b := make([]byte, buffersize)
				for {
					n, err := conn.Read(b)
					if err != nil {
						break
					}
					replyPkt(packet{p.connid, seqid, b[0:n]})
					seqid++
				}
			}()
		}
	}

	if !cpool.dupChk(p.connid, p.seqid) {
		u.conn.Write(p.buf)
	}
}

// send packed data to backend
func sendpkt(p packet) {
	u := upool.next()

	if u.conn == nil {
		// dial
		switch u.proto {
		case "tcp":
			conn, err := net.Dial("tcp", u.addr)
			if err != nil {
				// reply error
				replyRaw(packet{connid: p.connid})
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
					replyRaw(p)
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
		replyRaw(packet{connid: p.connid})
		return
	}
}

func replyRaw(p packet) {
	conn := cpool.get(p.connid)
	if conn == nil {
		log.Println("reply to no-exist client conn")
		return
	}
	if p.buf == nil {
		conn.Close()
		cpool.del(p.connid)
	} else {
		// get ride of duplicated connid+seqid
		// TODO: wait in case seqid is out of order
		if !cpool.dupChk(p.connid, p.seqid) {
			conn.Write(p.buf)
		}
	}
}

func replyPkt(p packet) {
	conn := epool.next()
	conn.Write(p)
}
