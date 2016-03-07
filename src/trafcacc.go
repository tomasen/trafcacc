package trafcacc

import (
	"encoding/gob"
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
	proto   string
	addr    string
	conn    net.Conn
	encoder gob.Encoder
	decoder gob.Decoder
}

var (
	isbackend = true
)

// Accelerate traffic start by flag strings
func Accelerate(l, u string, backend bool) {
	isbackend = backend

	for _, e := range parse(l) {
		// begin to listen
		for p := e.portBegin; p < e.portEnd; p++ {
			// listen to lhost:lport+p
			s := serv{proto: e.proto, addr: net.JoinHostPort(e.host, strconv.Itoa(p))}
			s.listen()
		}
	}

	for _, e := range parse(u) {
		for p := e.portBegin; p < e.portEnd; p++ {
			u := upstream{proto: e.proto, addr: net.JoinHostPort(e.host, strconv.Itoa(p))}
			upool.append(&u)
		}
	}

	if upool.end == 0 {
		log.Fatal("no upstreams")
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
		if isbackend {
			go s.hdlPacket(conn)
		} else {
			go s.hdlRaw(conn)
		}
	}
}

func (s *serv) hdlPacket(conn net.Conn) {
}

func (s *serv) hdlRaw(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("Recovered in", r, ":", string(debug.Stack()))
		}
	}()
	defer conn.Close()

	connid := atomic.AddUint32(&atomicid, 1)
	cpool.add(connid, conn)

	seqid := uint32(1)

	// send 0 length data to build connection
	sendpkt(packet{connid, seqid, []byte{}})

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
		sendpkt(packet{connid, seqid, buf})
	}
}
