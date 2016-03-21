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

	pqs *packetQueue
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
		go acceptTCP(ln, s.packetHandler)
	case udp:
		// TODO udp
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
			logrus.Warnln("packetHandler() Decode err:", err)
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
			go s.push(&p)
		}

	}
}

func (s *serv) push(p *packet) {

	if s.pqs.create(p.Senderid, p.Connid) {
		// it's new conn
		s.write(&packet{
			Senderid: p.Senderid,
			Connid:   p.Connid,
			Cmd:      connected,
		})

		s.handler.Serve(&conn{
			pconn: s.ServeMux,
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
		}).Warnln("unexpected Cmd in packet")
	}
}
