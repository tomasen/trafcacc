package trafcacc

import (
	"encoding/gob"
	"encoding/hex"
	"errors"
	"net"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

type cmd uint8

const (
	data cmd = iota
	close
	connect
)

type packet struct {
	Connid uint32
	Seqid  uint32
	Buf    []byte
	Cmd    cmd
}

// sendRaw only happens in backend to remote upstream addr
func (t *trafcacc) sendRaw(p packet) {
	log.WithFields(log.Fields{
		"connid": p.Connid,
		"seqid":  p.Seqid,
		"len":    len(p.Buf),
		"zdata":  shrinkString(hex.EncodeToString(p.Buf)),
	}).Debugln(t.roleString(), "sendRaw() to remote addr")

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
				log.WithFields(log.Fields{
					"connid": p.Connid,
					"error":  err,
				}).Debugln(t.roleString(), "Dial error")
				// reply error and close connection
				t.replyPkt(packet{Connid: p.Connid, Cmd: close})
				return
			}
			t.cpool.add(p.Connid, conn)
			go func() {
				log.Debugln(t.roleString(), "connected to remote begin to read")
				rname := "sendRawRead"
				routineAdd(rname)
				defer routineDel(rname)

				seqid := uint32(1)
				defer func() {
					log.WithFields(log.Fields{
						"connid": p.Connid,
						"seqid":  seqid,
					}).Debugln(t.roleString(), "remote connection closed")
					conn.Close()
					t.cpool.del(p.Connid)
					t.replyPkt(packet{Connid: p.Connid, Seqid: seqid, Cmd: close})
				}()
				b := make([]byte, buffersize)
				for {
					// break this loop when conn is closed
					conn.SetReadDeadline(time.Now().Add(time.Second))
					n, err := conn.Read(b)
					if err != nil {
						if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
							continue
						}
						log.WithFields(log.Fields{
							"connid": p.Connid,
							"error":  err,
						}).Debugln(t.roleString(), "remote connection read failed")
						break
					}
					conn.SetReadDeadline(time.Time{})
					err = t.replyPkt(packet{Connid: p.Connid, Seqid: seqid, Buf: b[0:n]})
					if err != nil {
						break
					}
					seqid++
				}
			}()
		}
	}

	t.pushToQueue(p, conn)
}

// send packed data to backend, only used on front-end
func (t *trafcacc) sendPkt(p packet) {
	log.WithFields(log.Fields{
		"connid": p.Connid,
		"seqid":  p.Seqid,
		"len":    len(p.Buf),
		"cmd":    p.Cmd,
		"zdata":  shrinkString(hex.EncodeToString(p.Buf)),
	}).Debugln(t.roleString(), "sendPkt()")

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
					// reply error and close connection to client
					log.WithFields(log.Fields{
						"connid": p.Connid,
					}).Debugln(t.roleString(), "dial in sendpkt() failed:", err)
					t.replyRaw(packet{Connid: p.Connid, Cmd: close})
					return
				}
				u.conn = conn
				u.encoder = gob.NewEncoder(conn)
				u.decoder = gob.NewDecoder(conn)
				// build packet reading slaves
				go func() {
					rname := "sendpktDecode"
					routineAdd(rname)
					defer routineDel(rname)

					defer u.close()

					u.mux.RLock()
					dec := u.decoder
					u.mux.RUnlock()
					for {
						p := packet{}
						err := dec.Decode(&p)
						if err != nil {
							log.WithFields(log.Fields{
								"connid": p.Connid,
								"error":  err,
							}).Debugln(t.roleString(), "read packet from backend failed")
							return
						}
						t.replyRaw(p)
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
		log.WithFields(log.Fields{
			"connid": p.Connid,
			"error":  err,
		}).Debugln(t.roleString(), "encode in sendpkt() failed")
		// reply error and close connection to client
		t.replyRaw(packet{Connid: p.Connid, Cmd: close})
		return
	}
}

// reply Raw only happens in front-end to client
func (t *trafcacc) replyRaw(p packet) {
	log.WithFields(log.Fields{
		"connid": p.Connid,
		"seqid":  p.Seqid,
		"cmd":    p.Cmd,
		"len":    len(p.Buf),
		"zdata":  shrinkString(hex.EncodeToString(p.Buf)),
	}).Debugln(t.roleString(), "replyRaw()")
	conn := t.cpool.get(p.Connid)

	if conn == nil {
		log.WithFields(log.Fields{
			"connid": p.Connid,
			"seqid":  p.Seqid,
			"len":    len(p.Buf),
			"cmd":    p.Cmd,
			"zdata":  shrinkString(hex.EncodeToString(p.Buf)),
		}).Debugln(t.roleString(), "reply to no-exist client conn")
		t.cpool.del(p.Connid)
		// TODO: do we need send close command here?
		// t.sendPkt(packet{Connid: p.Connid, Cmd: close})
	}
	t.pushToQueue(p, conn)
}

// reply Packet only happens in backend to frontend
func (t *trafcacc) replyPkt(p packet) error {
	log.WithFields(log.Fields{
		"connid": p.Connid,
		"seqid":  p.Seqid,
		"len":    len(p.Buf),
		"zdata":  shrinkString(hex.EncodeToString(p.Buf)),
	}).Debugln(t.roleString(), "replyPkt() to frontend")

	conn := t.epool.next()
	if conn != nil {
		return conn.Encode(p)
	}
	return errors.New("connection not exist anymore")
}

type pktQueue struct {
	lastseq uint32
	cond    *sync.Cond
	queue   map[uint32]*packet
}

func exists(queue map[uint32]*packet, seqid uint32) bool {
	_, ok := queue[seqid]
	return ok
}

// remove by connid from packet queue pool
func (t *trafcacc) removeQueue(connid uint32) {
	t.mux.Lock()
	defer t.mux.Unlock()
	pq := t.pq[connid]
	if pq != nil {
		delete(t.pq, connid)
	}
}

// ensure Write to client and remote in sequence
func (t *trafcacc) pushToQueue(p packet, conn net.Conn) {
	// TODO: just write if it's udp?
	t.mux.Lock()
	pq := t.pq[p.Connid]

	if pq == nil {
		pq = &pktQueue{
			cond:  sync.NewCond(&sync.Mutex{}),
			queue: make(map[uint32]*packet),
		}
		t.pq[p.Connid] = pq
		log.WithFields(log.Fields{
			"connid": p.Connid,
		}).Debugln(t.roleString(), "add new packet queue")

		go t.orderedWrite(pq, p.Connid, conn)
	}
	t.mux.Unlock()

	cond := pq.cond

	cond.L.Lock()
	pq.queue[p.Seqid] = &p
	log.WithFields(log.Fields{
		"connid": p.Connid,
		"seqid":  p.Seqid,
		"Cmd":    p.Cmd,
		"queue":  keysOfmap(pq.queue),
	}).Debugln(t.roleString(), "add new seq to queue")
	cond.L.Unlock()
	cond.Signal()
}

// ensure write order for this connid
func (t *trafcacc) orderedWrite(pq *pktQueue, connid uint32, conn net.Conn) {
	rname := "orderedWrite"
	routineAdd(rname)
	defer routineDel(rname)

	defer func() {
		log.WithFields(log.Fields{
			"connid": connid,
			"conn": conn,
		}).Debugln(t.roleString(), "packet queue exit")
		if conn != nil {
			conn.Close()
		}
		t.removeQueue(connid)
	}()

	cond := pq.cond
	for {
		cond.L.Lock()
		for !exists(pq.queue, pq.lastseq+1) {
			log.WithFields(log.Fields{
				"connid": connid,
				"seqid":  pq.lastseq + 1,
				"queue":  keysOfmap(pq.queue),
			}).Debugln(t.roleString(), "no new seq in the order")
			cond.Wait()
		}
		lastseq := pq.lastseq + 1

		log.WithFields(log.Fields{
			"connid":  connid,
			"lastseq": lastseq,
			"queue":   keysOfmap(pq.queue),
		}).Debugln(t.roleString(), "new seq packet is ready")

		cond.L.Unlock()

		for i := lastseq; ; i++ {
			cond.L.Lock()
			pkt, ok := pq.queue[i]
			cond.L.Unlock()
			if !ok {
				break
			} else {
				if pkt.Buf != nil {
					log.WithFields(log.Fields{
						"connid": pkt.Connid,
						"seqid":  pkt.Seqid,
						"len":    len(pkt.Buf),
						"zdata":  shrinkString(hex.EncodeToString(pkt.Buf)),
					}).Debugln(t.roleString(), "orderedWrite()")
					if conn == nil {
						log.Debugln(t.roleString(), "orderedWrite() connection already lost")
						return
					}
					_, err := conn.Write(pkt.Buf)
					if err != nil {
						// remove when connection closed
						log.Debugln(t.roleString(), "orderedWrite() err", err)
						return
					}
				}
				if pkt.Cmd == close {
					log.Debugln(t.roleString(), "orderedWrite() received close command")
					return
				}
				cond.L.Lock()
				pq.lastseq = i
				delete(pq.queue, i)
				cond.L.Unlock()
			}
		}

	}
}
