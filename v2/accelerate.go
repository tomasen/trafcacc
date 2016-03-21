package trafcacc

import (
	"io"
	"net"
	"strconv"

	"github.com/Sirupsen/logrus"
)

// Accelerate traffic by setup front-end dialer and back-end server
func Accelerate(l, u string, role tag) Trafcacc {
	t := &trafcacc{role: role}
	t.accelerate(l, u)
	return t
}

// tag is type of role: BACKEND or FRONTEND
type tag bool

// BACKEND FRONTEND tag the role that instance played with
const (
	BACKEND  tag = true
	FRONTEND tag = false
)

type trafcacc struct {
	role   tag
	remote *upstream
	dialer Dialer
}

// Trafcacc give a interface to query running status
type Trafcacc interface {
	Status()
	WaitforAlive()
}

func (t *trafcacc) Serve(conn net.Conn) {
	uc, err := net.Dial(t.remote.proto, t.remote.addr)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Errorln("backend dial error")
		return
	}
	defer conn.Close()

	ch := make(chan struct{}, 2)
	go pipe(conn, uc, ch)
	go pipe(uc, conn, ch)
	<-ch
	<-ch
}

// Accelerate traffic by listen to l, and connect to u
func (t *trafcacc) accelerate(l, u string) {
	switch t.role {
	case BACKEND:
		// TODO: setup trafcacc.Server to listen to and HandleFunc from l
		// connect to upstream
		for _, e := range parse(u) {
			for p := e.portBegin; p <= e.portEnd; p++ {
				t.remote = &upstream{proto: e.proto, addr: net.JoinHostPort(e.host, strconv.Itoa(p))}
				break
			}
		}
		if t.remote == nil {
			logrus.Fatalln("didn't specify remote addr for backend")
		}
		svr := NewServeMux()
		svr.Handle(l, t)

	case FRONTEND:
		// TODO: listen to l
		// use trafcacc.Dialer to init connection to u
		t.dialer = NewDialer()
		t.dialer.Setup(u)

		for _, e := range parse(l) {
			for p := e.portBegin; p <= e.portEnd; p++ {
				ln, err := net.Listen(e.proto, net.JoinHostPort(e.host, strconv.Itoa(p)))
				if err != nil {
					// handle error
					logrus.WithFields(logrus.Fields{
						"error":    err,
						"endpoint": e,
					}).Fatalln("frontend listen to address error")
				}

				go acceptTCP(ln, func(conn net.Conn) {
					up, err := t.dialer.Dial()
					if err != nil {
						// handle error
						logrus.WithFields(logrus.Fields{
							"error": err,
						}).Fatalln("frontend dial to address error")
					}
					defer up.Close()

					ch := make(chan struct{}, 2)
					go pipe(up, conn, ch)
					pipe(conn, up, ch)
					<-ch
					<-ch
				})
				break
			}
		}

	}
}

func (t *trafcacc) WaitforAlive() {
	// TODO:
}

func (t *trafcacc) roleString() string {
	switch t.role {
	case BACKEND:
		return "backend"
	case FRONTEND:
		return "frontend"
	}
	return "unknown"
}

// pipe upstream and downstream
func pipe(dst net.Conn, src net.Conn, ch chan struct{}) {
	defer func() {
		dst.Close()
		src.Close()
		ch <- struct{}{}
	}()

	/*
		b := make([]byte, buffersize)
		for {
			n, err := src.Read(b)
			if err != nil {
				logrus.Warnln("pipe read error", err)
				return
			}
			n, err = dst.Write(b[:n])
			if err != nil {
				logrus.Warnln("pipe write error", err)
				return
			}

		}
	*/
	_, err := io.Copy(dst, src)
	if err != nil {
		logrus.Warnln("pipe copy error", err)
		return
	}
}
