package trafcacc

import (
	"encoding/gob"
	"net"
	"strconv"
	"sync"

	"github.com/Sirupsen/logrus"
)

type serve struct {
	*sync.Cond
	*node

	alive   bool
	handler Handler
}

// HandlerFunc TODO: comment
type handlerFunc func(net.Conn)

// Serve TODO: comment
func (f handlerFunc) Serve(c net.Conn) {
	f(c)
}

// NewServe allocates and returns a new ServeMux.
func NewServe() Serve {
	return newServe()
}

func newServe() *serve {
	return &serve{
		Cond: sync.NewCond(&sync.Mutex{}),
		node: newNode("server"),
	}
}

// HandleFunc registers the handler for the given addresses
// that back-end server listened to
func (mux *serve) HandleFunc(listento string, handler func(net.Conn)) {
	mux.Handle(listento, handlerFunc(handler))
}

// Handle registers the handler for the given addresses
func (mux *serve) Handle(listento string, handler Handler) {
	// TODO: handle as backend
	mux.handler = handler
	for _, e := range parse(listento) {
		for p := e.portBegin; p <= e.portEnd; p++ {
			s := serv{serve: mux, proto: e.proto, addr: net.JoinHostPort(e.host, strconv.Itoa(p))}
			s.listen()
			go func() {
				s.waitforalive()
				mux.L.Lock()
				mux.alive = true
				mux.L.Unlock()
				mux.Broadcast()
			}()
		}
	}
}

func (mux *serve) waitforalive() {
	mux.L.Lock()
	for !mux.alive {
		mux.Wait()
	}
	mux.L.Unlock()
}

type serv struct {
	*serve
	proto string
	addr  string
	alive bool
}

func (s *serv) waitforalive() {
	s.L.Lock()
	for !s.alive {
		s.Wait()
	}
	s.L.Unlock()
}

func (s *serv) setalive() {
	s.L.Lock()
	s.alive = true
	s.L.Unlock()
	s.Broadcast()
}

func (s *serv) listen() {
	switch s.proto {
	case tcp:
		ln, err := net.Listen("tcp", s.addr)
		if err != nil {
			logrus.Fatalln("net.Listen error", s.addr, err)
		}

		s.setalive()

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

		s.setalive()

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
		udpbuf := make([]byte, buffersize)
		n, addr, err := conn.ReadFromUDP(udpbuf)
		if err != nil {
			logrus.WithError(err).Warnln("ReadFromUDP error")
			break
		}
		if u.udpaddr == nil {
			u.udpaddr = addr
		}

		p := packet{}
		if err := decodePacket(udpbuf[:n], &p); err != nil {
			logrus.WithError(err).Warnln("server gop decode from udp error", n)
			continue
		}

		p.udp = true
		if err := s.proc(&u, &p); err != nil {
			logrus.WithError(err).Warn("serve send pong err")
			return
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

		if err := s.proc(&u, &p); err != nil {
			logrus.WithError(err).Warn("serve send pong err")
			return
		}
	}
}

func (s *serv) proc(u *upstream, p *packet) error {
	s.node.proc(u, p)
	switch p.Cmd {
	case ping:
		// reply
		err := u.send(pong)
		if err != nil {
			return err
		}
	case data:
		go s.push(p)
	}
	return nil
}

func (s *serv) push(p *packet) {
	if s.pqs.create(p.Senderid, p.Connid) {
		// it's new conn
		s.write(&packet{
			Senderid: p.Senderid,
			Connid:   p.Connid,
			Cmd:      connected,
		})

		conn := newConn(s.serve, p.Senderid, p.Connid)

		s.handler.Serve(conn)
	}

	s.node.push(p)
}
