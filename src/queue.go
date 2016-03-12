package trafcacc

import (
  "net"
	"sync"
	"encoding/hex"

	log "github.com/Sirupsen/logrus"
)

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
  log.WithFields(log.Fields{
    "connid": connid,
  }).Debugln(t.roleString(), "remove packet queue")
	t.mux.Lock()
	defer t.mux.Unlock()
	_, ok := t.pq[connid]
	if ok {
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
    "lastseq": pq.lastseq,
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
				"waitseq":  pq.lastseq + 1,
				"queue":  keysOfmap(pq.queue),
			}).Debugln(t.roleString(), "wait for next seq id")
			cond.Wait()
		}

    lastseq := pq.lastseq + 1

    log.WithFields(log.Fields{
      "connid":  connid,
      "lastseq": lastseq,
      "queue":   keysOfmap(pq.queue),
    }).Debugln(t.roleString(), "new seq packet is ready to write")

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
        if pq.lastseq == 0 {
          log.Debugln(t.roleString(), "orderedWrite() set lastseq=0 might be issue")
        }
				delete(pq.queue, i)
				cond.L.Unlock()
			}
		}

	}
}
