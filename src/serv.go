package trafcacc

import (
	"encoding/gob"
	"io"
	"net"
	"sync/atomic"
	"time"

	log "github.com/Sirupsen/logrus"
)

type serv struct {
	proto string
	addr  string
	ln    net.Listener
	ta    *trafcacc
}

func (s *serv) listen() {
	switch s.proto {
	case "tcp":
		ln, err := net.Listen("tcp", s.addr)
		if err != nil {
			log.Fatal("net.Listen error", s.addr, err)
		}

		log.Debugln("listen to", s.addr)
		s.ln = ln
		go s.acceptTCP()
	case "udp":
		// TODO udp
	}
}

func (s *serv) acceptTCP() {
	rname := "acceptTCP"
	routineAdd(rname)
	defer routineDel(rname)

	defer s.ln.Close()
	var tempDelay time.Duration
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				time.Sleep(tempDelay)
				continue
			}
			log.Fatal(err)
		}
		tempDelay = 0

		switch s.ta.role {
		case BACKEND:
			go s.hdlPkt(conn)
		case FRONTEND:
			go s.hdlRaw(conn)
		}
	}
}

// handle packed data from client side as backend
func (s *serv) hdlPkt(conn net.Conn) {
	rname := "hdlPacket"
	routineAdd(rname)
	defer routineDel(rname)

	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)
	s.ta.epool.add(enc)
	defer func() {
		s.ta.epool.remove(enc)
		conn.Close()
	}()

	for {
		// TODO: avoid endless waiting?
		p := packet{}
		err := dec.Decode(&p)
		if err != nil {
			log.Debugln("hdlPacket err:", err)
			// TODO: just close or do some thing other?
			break
		}
		s.ta.sendRaw(p)
	}
}

// handle raw data from client side as front-end
func (s *serv) hdlRaw(conn net.Conn) {
	rname := "hdlRaw"
	routineAdd(rname)
	defer routineDel(rname)

	defer conn.Close()

	connid := atomic.AddUint32(&s.ta.atomicid, 1)
	s.ta.cpool.add(connid, conn)

	seqid := uint32(1)
	// send connect command to backend to estabilish connection
	s.ta.sendPkt(packet{Connid: connid, Seqid: seqid, Cmd: connect})

	defer func() {
		s.ta.sendPkt(packet{Connid: connid, Seqid: seqid + 1, Cmd: close})
		log.WithFields(log.Fields{
			"connid": connid,
		}).Debugln(s.ta.roleString(), "hdlRaw() exit")
		s.ta.cpool.del(connid)
	}()

	buf := make([]byte, buffersize)
	for {
		// TODO: close connection by packet command?
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Debugln(s.ta.roleString(), "read from client error:", err)
			}
			break
		} else {
			seqid++
			s.ta.sendPkt(packet{Connid: connid, Seqid: seqid, Buf: buf[:n]})
		}
	}
}
