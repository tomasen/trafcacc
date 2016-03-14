// serv.go 监听端口和分发收到的数据或数据包

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
	*trafcacc
	proto string
	addr  string
	ln    net.Listener
}

func (s *serv) listen() {
	switch s.proto {
	case "tcp":
		ln, err := net.Listen("tcp", s.addr)
		if err != nil {
			log.Fatal("net.Listen error", s.addr, err)
		}

		if log.GetLevel() >= log.DebugLevel {
			log.Debugln("listen to", s.addr)
		}
		s.ln = ln
		go s.acceptTCP()
	case "udp":
		// TODO udp
	}
}

func (s *serv) acceptTCP() {
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

		switch s.role {
		case BACKEND:
			go s.packetHandler(conn)
		case FRONTEND:
			go s.rawHandler(conn)
		}
	}
}

// handle packed data from client side as backend
func (s *serv) packetHandler(conn net.Conn) {
	const rname = "packetHandler"
	routineAdd(rname)
	defer routineDel(rname)

	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)
	s.epool.add(enc)
	defer func() {
		s.epool.remove(enc)
		conn.Close()
	}()

	for {
		// TODO: avoid endless waiting? without break the data packet
		// conn.SetReadDeadline(time.Now().Add(time.Second * decodetimeout))
		p := packet{}
		err := dec.Decode(&p)
		if err != nil {
			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				continue
			}
			if log.GetLevel() >= log.DebugLevel {
				log.Debugln("packetHandler() err:", err)
			}
			// just close it, and epool will wait for new connections
			break
		}
		s.sendRaw(p)
	}
}

// handle raw data from client side as front-end
func (s *serv) rawHandler(conn net.Conn) {
	const rname = "rawHandler"
	routineAdd(rname)
	defer routineDel(rname)

	defer conn.Close()

	connid := atomic.AddUint32(&s.atomicid, 1)
	s.cpool.add(connid, conn)

	seqid := uint32(1)
	// send connect command to backend to estabilish connection
	s.sendPkt(packet{Connid: connid, Seqid: seqid, Cmd: connect})

	defer func() {
		s.sendPkt(packet{Connid: connid, Seqid: seqid + 1, Cmd: close})
		if log.GetLevel() >= log.DebugLevel {
			log.WithFields(log.Fields{
				"connid": connid,
			}).Debugln(s.roleString(), "rawHandler() exit")
		}
		s.cpool.del(connid)
	}()

	buf := make([]byte, buffersize)
	for {
		// TODO: close connection by packet command?
		conn.SetReadDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				continue
			}
			if err != io.EOF {
				if log.GetLevel() >= log.DebugLevel {
					log.Debugln(s.roleString(), "read from client error:", err)
				}
			}
			break
		} else {
			seqid++
			s.sendPkt(packet{Connid: connid, Seqid: seqid, Buf: buf[:n]})
		}
	}
}
