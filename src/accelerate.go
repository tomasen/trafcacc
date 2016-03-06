package trafcacc

import (
	"fmt"
	"io"
	"log"
	"net"
	"runtime/debug"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	maxpacketqueue = 200000
	buffersize     = 4096 * 2
)

// client-->server
// client-->(serv-->trafcacc-->upstream)==>(serv-->trafcacc-->upstream) --> server
type endpoint struct {
	proto     string
	host      string
	portBegin int // port begin
	portEnd   int // port end
}

type serv struct {
	proto string
	addr  string
	ln    net.Listener
}

type upstream struct {
	proto string
	addr  string
	conns map[uint32]net.Conn
	next  *upstream
}

var (
	isbackend  = true
	isfrontend = false
)

// Accelerate traffic start by flag strings
func Accelerate(l, u string) {
	ee := uppack(l)
	if len(ee) == 1 && ee[0].portBegin == ee[0].portEnd {
		isbackend = false
		isfrontend = true
	}
	for _, e := range ee {

		// begin to listen
		for p := e.portBegin; p < e.portEnd; p++ {
			// listen to lhost:lport+p
			s := serv{proto: e.proto, addr: net.JoinHostPort(e.host, strconv.Itoa(p))}
			s.listen()
		}
	}

	var u0, pu *upstream
	for _, e := range uppack(u) {
		for p := e.portBegin; p < e.portEnd; p++ {
			u := upstream{proto: e.proto, addr: net.JoinHostPort(e.host, strconv.Itoa(p))}
			if pu != nil {
				pu.next = &u
			} else {
				u0 = &u
			}
			pu = &u
		}
	}
	if pu != nil {
		pu.next = u0
	} else {
		log.Fatal("no upstreams")
	}

	for {
		p := <-pktqueue
		go pu.sendpkt(p)
		pu = pu.next
	}
}

// MARK: serv
func (s *serv) listen() {
	switch s.proto {
	case "tcp":
		ln, err := net.Listen("tcp", s.addr)
		if err != nil {
			log.Fatal("net.Listen error", s.addr, err)
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
		go s.handleTCP(conn)
	}
}

func (s *serv) handleTCP(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("Recovered in", r, ":", string(debug.Stack()))
		}
	}()
	defer conn.Close()

	connid := atomic.AddUint32(&atomicid, 1)
	seqid := uint32(0)

	reply := make(chan packet, 1)
	go func() {
		for {
			p := <-reply
			if p.buf == nil {
				conn.Close()
				return
			}
			_, err := conn.Write(p.buf)
			if err != nil {
				log.Println("write to client error", p.connid, err)
				return
			}
		}
	}()

	// send 0 length data to build connection
	s.sendup(packet{connid, seqid, nil, reply})

	buf := make([]byte, buffersize)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				fmt.Println("read error:", err)
			}
			break
		}
		seqid++
		s.sendup(packet{connid, seqid, buf, reply})
	}
}
