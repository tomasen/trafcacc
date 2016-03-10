package trafcacc

import (
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"runtime/debug"
	"sync/atomic"
	"time"

	log "github.com/Sirupsen/logrus"
)

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
		log.Debugln("handler conn", s.ta.roleString())
		switch s.ta.role {
		case BACKEND:
			go s.hdlPacket(conn)
		case FRONTEND:
			go s.hdlRaw(conn)
		}
	}
}

// handle packed data from client side as backend
func (s *serv) hdlPacket(conn net.Conn) {
	rname := "hdlPacket"
	routineAdd(rname)
	defer routineDel(rname)

	log.Debugln("hdlPacket")
	//u.encoder = gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)
	s.ta.epool.add(enc)
	defer func() {
		s.ta.epool.remove(enc)
		conn.Close()
	}()

	var connid *uint32
	for {
		p := packet{}
		err := dec.Decode(&p)
		if err != nil {
			log.Debugln("hdlPacket err:", err)
			if connid != nil {
				s.ta.closeQueue(*connid)
			}
			break
		}
		connid = &p.Connid
		s.ta.sendRaw(p)
		if p.Seqid != 1 && p.Buf == nil {
			if connid != nil {
				s.ta.closeQueue(*connid)
			}
			return
		}
	}
}

// handle raw data from client side as front-end
func (s *serv) hdlRaw(conn net.Conn) {
	rname := "hdlRaw"
	routineAdd(rname)
	defer routineDel(rname)

	defer func() {
		if r := recover(); r != nil {
			log.Debugln("Recovered in", r, ":", string(debug.Stack()))
		}
	}()
	defer conn.Close()

	log.Debugln("hdlRaw", s)
	connid := atomic.AddUint32(&s.ta.atomicid, 1)
	s.ta.cpool.add(connid, conn)

	seqid := uint32(1)
	// send 0 length data to build connection
	s.ta.sendpkt(packet{connid, seqid, nil})

	defer func() {
		s.ta.sendpkt(packet{connid, seqid + 1, nil})
		s.ta.closeQueue(connid)
		s.ta.cpool.del(connid)
	}()

	buf := make([]byte, buffersize)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				fmt.Println("read error:", err)
			}
			break
		}
		seqid++
		s.ta.sendpkt(packet{connid, seqid, buf[:n]})
	}
}
