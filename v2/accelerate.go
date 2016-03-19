package trafcacc

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
)

// tag is type of role: BACKEND or FRONTEND
type tag bool

// BACKEND FRONTEND role tag
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
}

func (t *trafcacc) Serve(conn net.Conn) {
	uc, err := net.Dial(t.remote.proto, t.remote.addr)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Errorln("backend dial error")
		return
	}

	ch := make(chan struct{}, 2)
	go pipe(conn, uc, ch)
	go pipe(uc, conn, ch)
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

				go t.acceptTCP(ln)
				break
			}
		}

	}
}

func (t *trafcacc) waitforAlive() {
	// TODO:
}

func (t *trafcacc) acceptTCP(ln net.Listener) {
	defer ln.Close()
	var tempDelay time.Duration
	for {
		conn, err := ln.Accept()
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

		go func() {
			up, err := t.dialer.Dial()
			if err != nil {
				// handle error
				logrus.WithFields(logrus.Fields{
					"error": err,
				}).Fatalln("frontend dial to address error")
			}

			ch := make(chan struct{}, 2)
			go pipe(conn, up, ch)
			go pipe(up, conn, ch)
			<-ch
		}()
	}
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
func pipe(dst io.Writer, src io.Reader, ch chan struct{}) {
	defer func() {
		ch <- struct{}{}
	}()

	_, err := io.Copy(dst, src)

	fmt.Println("pipe end")
	switch err {
	case io.EOF:
		err = nil
		return
	case nil:
		return
	}
}
