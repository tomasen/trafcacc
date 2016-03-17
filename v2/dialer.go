package trafcacc

import (
	"encoding/gob"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
)

// Dialer is
type Dialer struct {
	pool []*upstream
	cond *sync.Cond
}

// NewDialer set
func NewDialer() *Dialer {
	return &Dialer{cond: sync.NewCond(&sync.Mutex{})}
}

// Dial acts like net.Dial
func (d *Dialer) Dial() (net.Conn, error) {
	return d.DialTimeout(time.Duration(0))
}

// DialTimeout is the maximum amount of time a dial will wait for
// a connect to complete. If Deadline is also set, it may fail
// earlier.
//
// The default is 0 means no timeout.
//
func (d *Dialer) DialTimeout(timeout time.Duration) (net.Conn, error) {
	// TODO: wait for upstream online
	conn := &conn{}

	ch := make(chan struct{}, 1)
	go func() {

	}()

	select {
	case <-ch:
	case <-time.After(timeout):
	}
	return conn, nil
}

// Setup upstream servers
func (d *Dialer) Setup(server string) {
	for _, e := range parse(server) {
		for p := e.portBegin; p <= e.portEnd; p++ {
			u := upstream{proto: e.proto, addr: net.JoinHostPort(e.host, strconv.Itoa(p))}
			d.pool = append(d.pool, &u)
			go d.connect(&u)
		}
	}
}

func (d *Dialer) connect(u *upstream) {
	for {
		switch u.proto {
		case "tcp":
			conn, err := net.Dial("tcp", u.addr)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"addr":  u.addr,
					"error": err,
				}).Warnln("Dialer dial upstream error")

				time.Sleep(time.Second)
				continue
			}

			u.encoder = gob.NewEncoder(conn)
			u.decoder = gob.NewDecoder(conn)

			// begin to ping
			err = u.ping()
			if err != nil {
				conn.Close()
				continue
			}

			u.conn = conn

			for {
				p := packet{}
				err = u.decoder.Decode(&p)
				if err != nil {
					logrus.WithFields(logrus.Fields{
						"error": err,
					}).Warnln("Dialer docode upstream packet")
					break
				}
				if p.Cmd == pong {
					err = u.ping()
					if err != nil {
						break
					}
				}

				d.push(&p)
			}
			conn.Close()
			u.conn = nil
		case "udp":
			// TODO: nothing?
		}
	}
}

func (d *Dialer) push(p *packet) {
	// TODO:
}

//
func (d *Dialer) alive() bool {
	for _, v := range d.pool {
		if v.conn != nil {
			return true
		}
	}
	return false
}
