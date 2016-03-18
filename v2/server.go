package trafcacc

import (
	"encoding/gob"
	"net"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
)

type packetQueue map[uint32]map[uint32]map[uint32]*packet
type closedQueue map[uint32]map[uint32]struct{}

// ServeMux TODO: comment
type ServeMux struct {
	// contains filtered or unexported fields
	handler Handler

	packetQueue packetQueue
	closedQueue closedQueue
}

// Handler TODO: comment
type Handler interface {
	Serve(net.Conn)
}

// HandlerFunc TODO: comment
type HandlerFunc func(net.Conn)

// Serve TODO: comment
func (f HandlerFunc) Serve(c net.Conn) {
	f(c)
}

// NewServeMux allocates and returns a new ServeMux.
func NewServeMux() *ServeMux {
	return &ServeMux{packetQueue: make(packetQueue)}
}

// DefaultServeMux TODO: comment
var DefaultServeMux = NewServeMux()

// HandleFunc registers the handler for the given addresses
// that back-end server listened to
func (mux *ServeMux) HandleFunc(listento string, handler func(net.Conn)) {
	mux.Handle(listento, HandlerFunc(handler))
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

func (mux *ServeMux) write(*packet) error {
	// TODO
	return nil
}

type serv struct {
	*ServeMux
	proto string
	addr  string
	ln    net.Listener
}

func (s *serv) listen() {
	switch s.proto {
	case "tcp":
		ln, err := net.Listen("tcp", s.addr)
		if err != nil {
			logrus.Fatalln("net.Listen error", s.addr, err)
		}

		if logrus.GetLevel() >= logrus.DebugLevel {
			logrus.Debugln("listen to", s.addr)
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
			logrus.Fatalln(err)
		}
		tempDelay = 0

		go s.packetHandler(conn)
	}
}

// handle packed data from client side as backend
func (s *serv) packetHandler(conn net.Conn) {
	defer conn.Close()

	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)

	for {
		p := packet{}
		err := dec.Decode(&p)
		if err != nil {
			if logrus.GetLevel() >= logrus.WarnLevel {
				logrus.Warnln("packetHandler() Decode err:", err)
			}
			break
		}
		switch p.Cmd {
		case ping:
			// reply ping
			err := enc.Encode(&packet{Cmd: pong})
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"error": err,
				}).Warnln("sever pong error")
			}
			continue

		default:
			s.push(&p)
		}

	}
}

func (s *serv) push(p *packet) {
	switch p.Cmd {

	case connect:
		if _, ok := s.packetQueue[p.Senderid][p.Connid]; !ok {
			delete(s.closedQueue[p.Senderid], p.Connid)

			// it's new conn
			p.Cmd = connected
			s.write(p)

			s.handler.Serve(&serverConn{
				ServeMux: s.ServeMux,
				senderid: p.Senderid,
				connid:   p.Connid,
			})
		}

	case close:
		delete(s.packetQueue[p.Senderid], p.Connid)
		s.closedQueue[p.Senderid][p.Connid] = struct{}{}

	default: //data
		if _, ok := s.closedQueue[p.Senderid][p.Connid]; !ok {
			if _, ok := s.packetQueue[p.Senderid][p.Connid][p.Seqid]; !ok {
				s.packetQueue[p.Senderid][p.Connid][p.Seqid] = p
			} // else drop duplicated packet
		} // else drop closed packet
	}
}
