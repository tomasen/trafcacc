// packet.go 数据包的收发，数据包与Raw数据之间的 Pack 和 Unpack 处理，Raw数据 的收发

package trafcacc

import (
	"encoding/gob"
	"encoding/hex"
	"errors"
	"net"
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

func (p packet) Copy() packet {
	r := packet{
		Connid: p.Connid,
		Seqid:  p.Seqid,
		Cmd:    p.Cmd,
	}

	if p.Buf != nil {
		r.Buf = make([]byte, buffersize)
		copy(r.Buf, p.Buf)
	}

	return r
}

// sendRaw only happens in backend to remote addr
func (t *trafcacc) sendRaw(p packet) {
	if t.cpool.shouldDrop(p.Connid) {
		if log.GetLevel() >= log.DebugLevel {
			log.WithFields(log.Fields{
				"connid": p.Connid,
			}).Debugln("drop packet for a closed connection")
		}
		return
	}
	if log.GetLevel() >= log.DebugLevel {
		log.WithFields(log.Fields{
			"connid": p.Connid,
			"seqid":  p.Seqid,
			"len":    len(p.Buf),
			"zdata":  shrinkString(hex.EncodeToString(p.Buf)),
		}).Debugln(t.roleString(), "sendRaw() to remote addr")
	}
	u := t.upool.next()

	// TODO: optimize this lock
	u.mux.Lock()
	defer u.mux.Unlock()
	// use cpool here , conn by connid
	conn := t.cpool.get(p.Connid)

	if conn == nil {
		var err error
		// dial
		switch u.proto {
		case "tcp":
			conn, err = dialTimeout("tcp", u.addr, time.Second*time.Duration(dialtimeout))
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

			go t.rawRead(p.Connid, conn)
		}
	}

	t.pushToQueue(p, conn)
}

// read from remote addr only happens in backend
func (t *trafcacc) rawRead(connid uint32, conn net.Conn) {
	if log.GetLevel() >= log.DebugLevel {
		log.Debugln(t.roleString(), "connected to remote begin to read")
	}
	const rname = "rawRead"
	routineAdd(rname)
	defer routineDel(rname)

	seqid := uint32(1)
	defer func() {
		if log.GetLevel() >= log.DebugLevel {
			log.WithFields(log.Fields{
				"connid": connid,
				"seqid":  seqid,
			}).Debugln(t.roleString(), "remote connection closed")
		}
		conn.Close()
		t.cpool.del(connid)
		t.replyPkt(packet{Connid: connid, Seqid: seqid, Cmd: close})
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
			if log.GetLevel() >= log.DebugLevel {
				log.WithFields(log.Fields{
					"connid": connid,
					"error":  err,
				}).Debugln(t.roleString(), "remote connection read failed")
			}
			break
		}
		conn.SetReadDeadline(time.Time{})
		err = t.replyPkt(packet{Connid: connid, Seqid: seqid, Buf: b[0:n]})
		if err != nil {
			break
		}
		seqid++
	}
}

// send packed data to backend, only used on front-end
func (t *trafcacc) sendPkt(p packet) {
	t.realSendPkt(p)
	t.realSendPkt(p)
}

func (t *trafcacc) realSendPkt(p packet) {

	u := t.upool.next()

	u.mux.Lock()
	defer u.mux.Unlock()
	if log.GetLevel() >= log.DebugLevel {
		log.WithFields(log.Fields{
			"connid": p.Connid,
			"seqid":  p.Seqid,
			"len":    len(p.Buf),
			"cmd":    p.Cmd,
			"zdata":  shrinkString(hex.EncodeToString(p.Buf)),
		}).Debugln(t.roleString(), "sendPkt()")
	}
	if u.conn == nil {
		// dial
		switch u.proto {
		case "tcp":
			conn, err := dialTimeout("tcp", u.addr, time.Second*time.Duration(dialtimeout))
			if err != nil {
				// reply error and close connection to client
				if log.GetLevel() >= log.DebugLevel {
					log.WithFields(log.Fields{
						"connid": p.Connid,
					}).Debugln(t.roleString(), "dial in sendpkt() failed:", err)
				}
				t.replyRaw(packet{Connid: p.Connid, Cmd: close})
				return
			}
			u.conn = conn
			u.encoder = gob.NewEncoder(conn)
			u.decoder = gob.NewDecoder(conn)
			// build packet reading slaves
			go t.pktDecode(u)
		case "udp":
			// TODO: udp
		}
	}

	err := u.encoder.Encode(&p)
	if err != nil {
		u.close()
		if log.GetLevel() >= log.DebugLevel {
			log.WithFields(log.Fields{
				"connid": p.Connid,
				"error":  err,
			}).Debugln(t.roleString(), "encode in sendpkt() failed")
		}
		// reply error and close connection to client
		t.replyRaw(packet{Connid: p.Connid, Cmd: close})
		return
	}
}

// read and decode packed data from backend, only used on front-end
func (t *trafcacc) pktDecode(u *upstream) {
	const rname = "pktDecode"
	routineAdd(rname)
	defer routineDel(rname)

	defer u.close()

	u.mux.RLock()
	dec := u.decoder
	u.mux.RUnlock()
	for {
		// u.conn.SetReadDeadline(time.Now().Add(time.Second * decodetimeout))
		p := packet{}
		err := dec.Decode(&p)
		if err != nil {
			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				continue
			}

			if log.GetLevel() >= log.DebugLevel {
				log.WithFields(log.Fields{
					"connid": p.Connid,
					"error":  err,
				}).Debugln(t.roleString(), "read packet from backend failed")
			}
			return
		}
		t.replyRaw(p)
	}
}

// reply Raw only happens in front-end to client
func (t *trafcacc) replyRaw(p packet) {
	if log.GetLevel() >= log.DebugLevel {
		log.WithFields(log.Fields{
			"connid": p.Connid,
			"seqid":  p.Seqid,
			"cmd":    p.Cmd,
			"len":    len(p.Buf),
			"zdata":  shrinkString(hex.EncodeToString(p.Buf)),
		}).Debugln(t.roleString(), "replyRaw()")
	}
	conn := t.cpool.get(p.Connid)

	if conn == nil {
		if log.GetLevel() >= log.DebugLevel {
			log.WithFields(log.Fields{
				"connid": p.Connid,
				"seqid":  p.Seqid,
				"len":    len(p.Buf),
				"cmd":    p.Cmd,
				"zdata":  shrinkString(hex.EncodeToString(p.Buf)),
			}).Debugln(t.roleString(), "reply to no-exist client conn")
		}
		t.cpool.del(p.Connid)
		// TODO: do we need send close command here?
		// t.sendPkt(packet{Connid: p.Connid, Cmd: close})
	}
	t.pushToQueue(p, conn)
}

// reply Packet only happens in backend to frontend
func (t *trafcacc) replyPkt(p packet) (err error) {
	err = t.realReplyPkt(p)
	if err != nil {
		return err
	}
	err = t.realReplyPkt(p)
	if err != nil {
		return err
	}

	return nil
}

func (t *trafcacc) realReplyPkt(p packet) error {
	if log.GetLevel() >= log.DebugLevel {
		log.WithFields(log.Fields{
			"connid": p.Connid,
			"seqid":  p.Seqid,
			"len":    len(p.Buf),
			"zdata":  shrinkString(hex.EncodeToString(p.Buf)),
		}).Debugln(t.roleString(), "replyPkt() to frontend")
	}
	conn := t.epool.next()
	if conn != nil {
		return conn.Encode(p)
	}
	return errors.New("connection not exist anymore")
}
