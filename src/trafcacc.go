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
	maxpacketqueue      = 200000
	buffersize          = 96
	_BackendDialTimeout = 5
)

type serv struct {
	proto string
	addr  string
	ln    net.Listener
	ta    *trafcacc
}

type trafcacc struct {
	isbackend bool
	atomicid  uint32
	cpool     *poolc
	upool     *poolu
	epool     *poole

	// packet queue
	pq map[uint32]*pktQueue
}

// Accelerate traffic by setup listening port and upstream
func Accelerate(l, u string, backend bool) {
	t := &trafcacc{}
	t.cpool = newPoolc()
	t.upool = &poolu{}
	t.epool = &poole{}
	t.pq = make(map[uint32]*pktQueue)
	t.accelerate(l, u, backend)
}

// Accelerate traffic start by flag strings
func (t *trafcacc) accelerate(l, u string, backend bool) {
	// TODO: make sure this only run once
	t.isbackend = backend

	for _, e := range parse(u) {
		for p := e.portBegin; p <= e.portEnd; p++ {
			u := upstream{proto: e.proto, addr: net.JoinHostPort(e.host, strconv.Itoa(p))}
			t.upool.append(&u)
		}
	}

	if t.upool.end == 0 {
		log.Fatal("no upstreams")
	}

	for _, e := range parse(l) {
		// begin to listen
		for p := e.portBegin; p <= e.portEnd; p++ {
			// listen to lhost:lport+p
			s := serv{proto: e.proto, addr: net.JoinHostPort(e.host, strconv.Itoa(p)), ta: t}
			s.listen()
		}
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

		log.Println("listen to", s.addr)
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
		log.Println("handler conn", s.ta.isbackend)
		if s.ta.isbackend {
			go s.hdlPacket(conn)
		} else {
			go s.hdlRaw(conn)
		}
	}
}

// handle packed data from client side as backend
func (s *serv) hdlPacket(conn net.Conn) {
	log.Println("hdlPacket")
	//u.encoder = gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)
	id := s.ta.epool.add(enc)
	defer s.ta.epool.remove(id)

	for {
		p := packet{}
		err := dec.Decode(&p)
		if err != nil {
			log.Println("hdlPacket err:", err)
			break
		}
		s.ta.sendRaw(p)
	}
}

// handle raw data from client side as front-end
func (s *serv) hdlRaw(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("Recovered in", r, ":", string(debug.Stack()))
		}
	}()
	defer conn.Close()

	log.Println("hdlRaw", s)
	connid := atomic.AddUint32(&s.ta.atomicid, 1)
	log.Println("hdlRaw0")
	s.ta.cpool.add(connid, conn)
	log.Println("hdlRaw1")
	seqid := uint32(1)
	log.Println("hdlRaw2")
	// send 0 length data to build connection
	s.ta.sendpkt(packet{connid, seqid, []byte{}})
	log.Println("hdlRaw3")
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
