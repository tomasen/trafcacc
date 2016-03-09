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

// sendRaw only happens in backend to remote upstream addr
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

	t.ensure(p, conn)
}

// send packed data to backend
func (t *trafcacc) sendpkt(p packet) {
	log.Println("sendpkt", t.isbackend, p.Connid, p.Seqid, len(p.Buf), hex.EncodeToString(p.Buf), t.upool)
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

// reply Raw only happens in front-end to client
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
		t.closeQueue(p.Connid)
	} else {
		t.ensure(p, conn)
	}
}

func (t *trafcacc) replyPkt(p packet) {
	log.Println("replyPkt", t.isbackend, p.Connid, p.Seqid, len(p.Buf), hex.EncodeToString(p.Buf))
	conn := t.epool.next()
	conn.Encode(p)
}

type pktQueue struct {
	lastseq uint32
	cond    *sync.Cond
	queue   map[uint32][]byte
	closed  bool
}

func exists(queue map[uint32][]byte, seqid uint32) bool {
	_, ok := queue[seqid]
	return ok
}

func (t *trafcacc) closeQueue(connid uint32) {
	pq := t.pq[connid]
	if pq != nil {
		pq.closed = true
		delete(t.pq, connid)
	}
}

// ensure Write in sequence
func (t *trafcacc) ensure(p packet, conn net.Conn) {
	// TODO: just write if it's udp
	t.mux.RLock()
	pq := t.pq[p.Connid]
	t.mux.RUnlock()

	if pq == nil {
		// TODO: remove all these when connection closed
		pq = &pktQueue{cond: sync.NewCond(&sync.Mutex{}), queue: make(map[uint32][]byte)}

		t.mux.Lock()
		t.pq[p.Connid] = pq
		t.mux.Unlock()

		go func() {
			cond := pq.cond
			for {
				cond.L.Lock()
				for !exists(pq.queue, pq.lastseq+1) && !pq.closed {
					log.Println(t.isbackend, "not exist", pq.queue, pq.lastseq+1)
					cond.Wait()
				}

				log.Println(t.isbackend, "is exist", pq.queue, pq.lastseq+1)

				for i := pq.lastseq + 1; ; i++ {
					buf, ok := pq.queue[i]
					if ok {
						if buf != nil {
							_, err := conn.Write(buf)
							if err != nil {
								t.closeQueue(p.Connid)
								break
							}
						}
						pq.lastseq = i
						delete(pq.queue, i)
					} else {
						break
					}
				}
				cond.L.Unlock()
			}
		}()
	}

	cond := pq.cond

	cond.L.Lock()
	pq.queue[p.Seqid] = p.Buf
	cond.L.Unlock()
	cond.Signal()
}
