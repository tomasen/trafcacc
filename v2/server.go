package trafcacc

import (
	"bytes"
	"encoding/gob"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
)

// ServeMux TODO: comment
type ServeMux struct {
	// contains filtered or unexported fields
	handler Handler
	pool    *streampool
	pqs     *packetQueue
}

// Handler TODO: comment
type Handler interface {
	Serve(net.Conn)
}

// HandlerFunc TODO: comment
type handlerFunc func(net.Conn)

// Serve TODO: comment
func (f handlerFunc) Serve(c net.Conn) {
	f(c)
}

// NewServeMux allocates and returns a new ServeMux.
func NewServeMux() *ServeMux {
	return &ServeMux{
		pqs:  newPacketQueue(),
		pool: newStreamPool(),
	}
}

// HandleFunc registers the handler for the given addresses
// that back-end server listened to
func (mux *ServeMux) HandleFunc(listento string, handler func(net.Conn)) {
	mux.Handle(listento, handlerFunc(handler))
}

// Handle registers the handler for the given addresses
func (mux *ServeMux) Handle(listento string, handler Handler) {
	// TODO: handle as backend
	mux.handler = handler
	for _, e := range parse(listento) {
		for p := e.portBegin; p <= e.portEnd; p++ {
			s := serv{ServeMux: mux, proto: e.proto, addr: net.JoinHostPort(e.host, strconv.Itoa(p))}
			s.listen()
		}
	}
}

func (mux *ServeMux) pq() *packetQueue {
	return mux.pqs
}

func (mux *ServeMux) role() string {
	return "server"
}

type serv struct {
	*ServeMux
	proto string
	addr  string
}

func (s *serv) listen() {
	switch s.proto {
	case tcp:
		ln, err := net.Listen("tcp", s.addr)
		if err != nil {
			logrus.Fatalln("net.Listen error", s.addr, err)
		}

		if logrus.GetLevel() >= logrus.DebugLevel {
			logrus.Debugln("listen to", s.addr)
		}
		go acceptTCP(ln, s.tcphandler)
	case udp:
		udpaddr, err := net.ResolveUDPAddr("udp", s.addr)
		if err != nil {
			logrus.Fatalln("net.ResolveUDPAddr error", s.addr, err)
		}
		udpconn, err := net.ListenUDP("udp", udpaddr)
		if err != nil {
			logrus.Fatalln("net.ListenUDP error", udpaddr, err)
		}
		go func() {
			for {
				s.udphandler(udpconn)
			}
		}()
	}
}

func (s *serv) udphandler(conn *net.UDPConn) {
	u := upstream{
		proto:   s.proto,
		udpconn: conn,
	}
	// add to pool
	s.pool.append(&u, 0)
	defer func() {
		u.close()
		s.pool.remove(&u)
	}()

	for {
		udpbuf := udpBufferPool.Get().([]byte)
		defer udpBufferPool.Put(udpbuf)
		n, addr, err := conn.ReadFromUDP(udpbuf)
		if err != nil {
			logrus.WithError(err).Warnln("ReadFromUDP error")
			break
		}
		if u.udpaddr == nil {
			u.udpaddr = addr
		}

		p := packet{}
		if err := gob.NewDecoder(bytes.NewReader(udpbuf[:n])).Decode(&p); err != nil {
			logrus.WithError(err).Warnln("server gop decode from udp error", n)
			continue
		}

		atomic.AddUint64(&u.recv, uint64(len(p.Buf)))

		switch p.Cmd {
		case ping:
			atomic.StoreInt64(&u.alive, time.Now().UnixNano())
			s.pool.Broadcast()

			// reply
			err := u.send(pong)
			if err != nil {
				return
			}
			continue
		default:
			go s.push(&p)
		}
	}
}

// handle packed data from client side as backend
func (s *serv) tcphandler(conn net.Conn) {

	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)

	// add to pool
	u := upstream{
		proto:   s.proto,
		encoder: enc,
		decoder: dec,
	}

	defer func() {
		conn.Close()
		// remove from pool
		s.pool.remove(&u)
	}()

	s.pool.append(&u, 0)

	for {
		p := packet{}
		err := dec.Decode(&p)
		if err != nil {
			logrus.Warnln("packetHandler() Decode err:", err)
			break
		}
		atomic.AddUint64(&u.recv, uint64(len(p.Buf)))

		switch p.Cmd {
		case ping:
			atomic.StoreInt64(&u.alive, time.Now().UnixNano())
			s.pool.Broadcast()

			// reply
			err := u.send(pong)
			if err != nil {
				break
			}
			continue
		default:
			go s.push(&p)
		}
	}
}

func (mux *ServeMux) write(p *packet) error {

	return mux.pool.write(p)
}

func (s *serv) push(p *packet) {
	if s.pqs.create(p.Senderid, p.Connid) {
		// it's new conn
		s.write(&packet{
			Senderid: p.Senderid,
			Connid:   p.Connid,
			Cmd:      connected,
		})

		s.handler.Serve(&packetconn{
			pconn:    s.ServeMux,
			senderid: p.Senderid,
			connid:   p.Connid,
		})
	}

	switch p.Cmd {

	case connect:
	case close, data:
		s.pqs.add(p)
	default:
		logrus.WithFields(logrus.Fields{
			"Cmd": p.Cmd,
		}).Warnln("unexpected Cmd in packet on server", closed)
	}
}
