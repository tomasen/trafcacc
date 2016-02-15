package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	maxpacketqueue = 200000
	buffersize     = 4096 * 2
	maxopenfile    = 3267600
)

// client-->server
// client-->(serv-->traicacc-->upstream)==>(serv-->traicacc-->upstream) --> server
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
	tcp   net.Conn
	udp   net.UDPConn
	next  *upstream
}

type packet struct {
	connid uint64
	seqid  uint32
	buf    []byte
	reply  chan packet
}

var (
	atomicid uint64
	pktqueue = make(chan packet, maxpacketqueue)
)

func main() {
	// -listen=tcp://:500 -upstream=udp://172.0.0.1:2000-2100
	// -listen=udp://:2000-2100 -upstream=tcp://172.0.0.1:500
	listen := flag.String("listen", "<proto>://<ip>:<port begin-end>[,...] eg. udp://0.0.0.0:500", "listen to")
	upstream := flag.String("upstream", "<proto>://<ip>:<port begin-end>[,...] eg. udp://172.0.0.1:2000-2100,192.168.1.1:2000-2050", "send to")

	flag.Parse()

	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		log.Fatal("failed to get NOFILE rlimit: ", err)
	}

	lim.Cur = maxopenfile
	lim.Max = maxopenfile

	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		log.Fatal("failed to set NOFILE rlimit: ", err)
	}

	runtime.GOMAXPROCS(runtime.NumCPU())

	go accelerate(*listen, *upstream)
}

func accelerate(l, u string) {
	for _, e := range uppack(l) {
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

func uppack(s string) (e []endpoint) {
	x := strings.Split(s, ",")
	if len(x) < 1 {
		log.Fatal("argument error", s)
	}

	for _, s0 := range x {
		e0 := endpoint{}
		x0 := strings.Split(s0, "://")
		if len(x0) < 2 {
			log.Fatal("argument error", s0)
		}
		switch x0[0] {
		case "tcp":
		case "udp":
		default:
			log.Fatal("unknown proto:", x0[0])
		}
		e0.proto = x0[0]
		h, p, err := net.SplitHostPort(x0[1])
		if err != nil {
			log.Fatal("argument", x0[1], "error:", err)
		}
		e0.host = h

		x1 := strings.Split(p, "-")
		if len(x1) < 1 {
			log.Fatal("argument port", p, "error:", err)
		}

		e0.portBegin, err = strconv.Atoi(x1[0])
		if err != nil {
			log.Fatal("argument port begin ", x1[0], "error:", err)
		}

		if len(x1) < 2 {
			e0.portEnd = e0.portBegin
		} else {
			e0.portEnd, err = strconv.Atoi(x1[1])
			if err != nil {
				log.Fatal("argument port end ", x1[1], "error:", err)
			}
		}

		e = append(e, e0)
	}
	return e
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
		conn.Close()
		if r := recover(); r != nil {
			log.Println("Recovered in", r, ":", string(debug.Stack()))
		}
	}()

	connid := atomic.AddUint64(&atomicid, 1)
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

// send to upstream
func (s *serv) sendup(p packet) {
	select {
	case pktqueue <- p:
	default:
	}
}

// MARK: upstream
func (u *upstream) sendpkt(p packet) {
	// TODO: 是否建立新连接基于:
	// ??? 是否 buf 是 nil
	// 是否已经建立过连接，没有则必然建立
	// 是新的connid一定就是要建立新连接
	// 是 front 还是 backend
	// backend 的话，不是新的connid就使用上一个 connid 的连接

	if p.buf == nil {
		// dial
		switch u.proto {
		case "tcp":
			conn, err := net.Dial("tcp", u.addr)
			if err != nil {
				// reply error
				p.reply <- p
				return
			}
			u.tcp = conn
		case "udp":
			// TODO
		}
		return
	}
}
