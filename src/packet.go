package trafcacc

import (
	"encoding/gob"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"sync"
	"sync/atomic"
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
				rname := "sendRawRead"
				routineAdd(rname)
				defer routineDel(rname)

				defer func() {
					conn.Close()
					t.cpool.del(p.Connid)
					t.replyPkt(packet{p.Connid, p.Seqid, nil})
				}()
				seqid := uint32(1)
				b := make([]byte, buffersize)
				for {
					n, err := conn.Read(b)
					if err != nil {
						break
					}
					err = t.replyPkt(packet{p.Connid, seqid, b[0:n]})
					if err != nil {
						break
					}
					seqid++
				}
			}()
		}
	}

	t.ensure(p, conn)
}

// send packed data to backend, only used on front-end
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
					rname := "sendpktDecode"
					routineAdd(rname)
					defer routineDel(rname)

					defer func() {
						u.close()
					}()
					u.mux.RLock()
					dec := u.decoder
					u.mux.RUnlock()
					for {
						p := packet{}
						err := dec.Decode(&p)
						if err != nil {
							return
						}
						t.replyRaw(p)
						if p.Seqid != 1 && p.Buf == nil {
							return
						}
					}
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
	defer t.closeQueue(p.Connid)
	if conn == nil {
		log.Println("reply to no-exist client conn", p)
		t.cpool.del(p.Connid)
		return
	}
	if p.Seqid != 1 && p.Buf == nil {
		log.Println("replyRaw close:", p)
		conn.Close()
		t.cpool.del(p.Connid)
	} else {
		t.ensure(p, conn)
	}
}

func (t *trafcacc) replyPkt(p packet) error {
	log.Println("replyPkt", t.isbackend, p.Connid, p.Seqid, len(p.Buf), hex.EncodeToString(p.Buf))
	conn := t.epool.next()
	if conn != nil {
		return conn.Encode(p)
	}
	return errors.New("connection not exist anymore")
}

type pktQueue struct {
	lastseq uint32
	cond    *sync.Cond
	queue   map[uint32][]byte
	closed  int32
}

func exists(queue map[uint32][]byte, seqid uint32) bool {
	_, ok := queue[seqid]
	return ok
}

func (t *trafcacc) closeQueue(connid uint32) {
	t.mux.Lock()
	defer t.mux.Unlock()
	pq := t.pq[connid]
	if pq != nil {
		atomic.StoreInt32(&pq.closed, 1)
		pq.cond.Signal()
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
		pq = &pktQueue{cond: sync.NewCond(&sync.Mutex{}), queue: make(map[uint32][]byte)}

		t.mux.Lock()
		t.pq[p.Connid] = pq
		t.mux.Unlock()

		go func() {
			rname := "ensureCond"
			routineAdd(rname)
			defer routineDel(rname)

			defer func() {
				log.Println("exit cond")
				conn.Close()
				t.closeQueue(p.Connid)
			}()
			cond := pq.cond
			for {
				if atomic.LoadInt32(&pq.closed) != 0 {
					return
				}

				cond.L.Lock()
				for !exists(pq.queue, pq.lastseq+1) && atomic.LoadInt32(&pq.closed) == 0 {
					log.Println(t.isbackend, "not exist", pq.queue, pq.lastseq+1)
					cond.Wait()
				}
				log.Println(t.isbackend, "is exist", pq.queue, pq.lastseq+1)
				lastseq := pq.lastseq + 1
				cond.L.Unlock()

				for i := lastseq; ; i++ {
					cond.L.Lock()
					buf, ok := pq.queue[i]
					cond.L.Unlock()
					if ok {
						if buf != nil {
							_, err := conn.Write(buf)
							if err != nil {
								// remove when connection closed
								log.Println("cond write err", err)
								atomic.StoreInt32(&pq.closed, 1)
								break
							}
						} else if i > 1 {
							log.Println("cond write closed")
							atomic.StoreInt32(&pq.closed, 1)
							break
						}
						cond.L.Lock()
						pq.lastseq = i
						delete(pq.queue, i)
						cond.L.Unlock()
					} else {
						break
					}
				}

			}
		}()
	}

	cond := pq.cond

	cond.L.Lock()
	pq.queue[p.Seqid] = p.Buf
	cond.L.Unlock()
	cond.Signal()
}
