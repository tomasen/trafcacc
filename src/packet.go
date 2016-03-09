package trafcacc

import (
	"encoding/gob"
	"encoding/hex"
	"log"
	"net"
	"sync"
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
		t.pq.ensure(p, conn)
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
			t.pq.ensure(p, conn)
		}()
	}
}

func (t *trafcacc) replyPkt(p packet) {
	log.Println("replyPkt", t.isbackend, p.Connid, p.Seqid, len(p.Buf), hex.EncodeToString(p.Buf))
	conn := t.epool.next()
	conn.Encode(p)
}

// TODO: track all connections
const maxtrackconn = uint32(^uint16(0))

type packetQueue struct {
	lastseq [maxtrackconn]uint32
	sedcond [maxtrackconn]*sync.Cond
	ta      *trafcacc
}

// ensure Write in sequence
func (p *packetQueue) ensure(pkt packet, conn net.Conn) {
	// TODO: mutex?
	idx := pkt.Connid & maxtrackconn
	lastseq := p.lastseq[idx]

	cond := p.sedcond[idx]
	if cond == nil {
		cond = sync.NewCond(&sync.Mutex{})
		p.sedcond[idx] = cond
		log.Println(p.ta.isbackend, "new cond", pkt.Connid)
	}
	defer func() {
		cond.L.Lock()
		p.lastseq[idx] = pkt.Seqid
		cond.L.Unlock()
		cond.Broadcast()
		log.Println(p.ta.isbackend, "broadcast", pkt.Connid, pkt.Seqid, cond)
	}()

	if pkt.Buf == nil {
		// TODO: close connection?
		return
	}

	log.Println(p.ta.isbackend, "ensure", pkt.Connid, pkt.Seqid, lastseq)
	switch {
	case pkt.Seqid <= lastseq:
		// get ride of duplicated connid+seqid
		return
	case pkt.Seqid == lastseq+1:
		conn.Write(pkt.Buf)
		return
	default:
		// wait if case seqid is out of order
		cond.L.Lock()
		for p.lastseq[idx]+1 != pkt.Seqid {
			if pkt.Seqid <= p.lastseq[idx] {
				// get ride of duplicated connid+seqid
				log.Println(p.ta.isbackend, "get ride2", pkt.Connid, pkt.Seqid, p.lastseq[idx], cond)
				cond.L.Unlock()
				return
			}
			log.Println(p.ta.isbackend, "wait seq1", pkt.Connid, pkt.Seqid, p.lastseq[idx], cond)
			cond.Wait()
			log.Println(p.ta.isbackend, "wait seq2", pkt.Connid, pkt.Seqid, p.lastseq[idx], cond)
		}
		cond.L.Unlock()
		conn.Write(pkt.Buf)
		return
	}
}
