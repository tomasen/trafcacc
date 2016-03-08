package trafcacc

import (
	"encoding/gob"
	"encoding/hex"
	"log"
	"net"
)

type packet struct {
	Connid uint32
	Seqid  uint32
	Buf    []byte
}

func (t *trafcacc) sendRaw(p packet) {
	log.Println("sendRaw", t.isbackend, p.Connid, p.Seqid, len(p.Buf), hex.EncodeToString(p.Buf))
	// use cpool here , conn by connid
	conn := t.cpool.get(p.Connid)
	u := t.upool.next()

	if conn == nil {
		var err error
		// dial
		switch u.proto {
		case "tcp":
			conn, err = net.Dial("tcp", u.addr)
			if err != nil {
				// reply error
				t.replyPkt(packet{Connid: p.Connid})
				return
			}
			t.cpool.add(p.Connid, conn)
			go func() {
				seqid := uint32(1)
				b := make([]byte, buffersize)
				for {
					n, err := conn.Read(b)
					if err != nil {
						break
					}
					t.replyPkt(packet{p.Connid, seqid, b[0:n]})
					seqid++
				}
			}()
		}
	}

	go func() {
		t.cpool.ensure(p, conn)
	}()
}

// send packed data to backend
func (t *trafcacc) sendpkt(p packet) {
	log.Println("sendpkt", t.isbackend, p.Connid, p.Seqid, len(p.Buf), hex.EncodeToString(p.Buf))
	u := t.upool.next()

	func() {
		u.mux.Lock()
		defer u.mux.Unlock()
		if u.conn == nil {
			// dial
			switch u.proto {
			case "tcp":
				conn, err := net.Dial("tcp", u.addr)
				if err != nil {
					// reply error
					t.replyRaw(packet{Connid: p.Connid})
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
					u.close()
				}()
			case "udp":
				// TODO: udp
			}
		}
	}()

	err := u.encoder.Encode(&p)
	if err != nil {
		u.close()
		log.Println("sendpkt err:", err)
		// reply error
		t.replyRaw(packet{Connid: p.Connid})
		return
	}
}

func (t *trafcacc) replyRaw(p packet) {
	log.Println("replyRaw", t.isbackend, p.Connid, p.Seqid, len(p.Buf), hex.EncodeToString(p.Buf))
	conn := t.cpool.get(p.Connid)

	if conn == nil {
		log.Println("reply to no-exist client conn")
		return
	}
	if p.Buf == nil {
		conn.Close()
		t.cpool.del(p.Connid)
	} else {
		go func() {
			t.cpool.ensure(p, conn)
		}()
	}
}

func (t *trafcacc) replyPkt(p packet) {
	log.Println("replyPkt", t.isbackend, p.Connid, p.Seqid, len(p.Buf), hex.EncodeToString(p.Buf))
	conn := t.epool.next()
	conn.Encode(p)
}
