package trafcacc

import (
	"encoding/gob"
	"errors"
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

	pool *streampool

	packetQueue *packetQueue
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
	return &ServeMux{
		packetQueue: newPacketQueue(),
		pool:        newStreamPool(),
	}
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

func (mux *ServeMux) write(p *packet) error {
	successed := false

	// TODO: wait for upstreams avalible?

	// pick upstream tunnel and send packet
	for _, u := range mux.pool.pickupstreams() {
		err := u.encoder.Encode(&p)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Warnln("Server encode packet to upstream error")
		} else {
			successed = true
		}
	}
	if successed {
		// return error if all failed
		return nil
	}
	logrus.WithFields(logrus.Fields{
		"error": "no successed write",
	}).Warnln("Server encode packet to upstream error")
	return errors.New("dialer encoder error")
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

	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)

	// add to pool
	u := upstream{
		proto:   "tcp",
		addr:    conn.RemoteAddr().String(),
		encoder: enc,
		decoder: dec,
	}

	defer func() {
		conn.Close()
		// remove from pool
		s.pool.remove(u.proto, u.addr)
	}()

	s.pool.append(&u)

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
			atomic.StoreInt64(&u.alive, time.Now().UnixNano())
			s.pool.Broadcast()

			// reply
			err := u.send(pong)
			if err != nil {
				break
			}
			continue
		default:
			s.push(&p)
		}

	}
}

func (s *serv) push(p *packet) {
	if s.packetQueue.create(p.Senderid, p.Connid) {
		// it's new conn
		s.write(&packet{
			Senderid: p.Senderid,
			Connid:   p.Connid,
			Cmd:      connected,
		})

		s.handler.Serve(&serverConn{
			ServeMux: s.ServeMux,
			senderid: p.Senderid,
			connid:   p.Connid,
		})
	}

	switch p.Cmd {

	case connect:
	case close:
		fallthrough
	default: //data
		s.packetQueue.add(p)
	}
}
