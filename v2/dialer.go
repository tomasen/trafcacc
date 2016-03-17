package trafcacc

import (
	"encoding/gob"
	"errors"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
)

// Dialer is TODO: comment
type Dialer struct {
	keepalive time.Duration

	pool []*upstream
	cond *sync.Cond

	atomicid uint32
}

// NewDialer TODO: comment
func NewDialer() *Dialer {
	return &Dialer{cond: sync.NewCond(&sync.Mutex{}),
		keepalive: time.Second * 3,
	}
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
	// wait for upstream online and alive
	ch := make(chan struct{}, 1)
	go func() {
		d.cond.L.Lock()
		for !d.check() {
			d.cond.Wait()
		}
		d.cond.L.Unlock()

		// TODO: send connect cmd and wait for connected cmd
		ch <- struct{}{}
	}()

	select {
	case <-ch:
	case <-time.After(timeout):
		return nil, errors.New("i/o timeout")
	}

	conn := &conn{
		Dialer: d,
		connid: atomic.AddUint32(&d.atomicid, 1),
	}

	d.write(&packet{
		Connid: conn.connid,
		Cmd:    connect,
	})

	return conn, nil
}

func (d *Dialer) write(p *packet) {
	// TODO: pick one upstream tunnel and send packet
}

// Setup upstream servers
func (d *Dialer) Setup(server string) {
	for _, e := range parse(server) {
		for p := e.portBegin; p <= e.portEnd; p++ {
			u := upstream{proto: e.proto, addr: net.JoinHostPort(e.host, strconv.Itoa(p))}
			d.cond.L.Lock()
			d.pool = append(d.pool, &u)
			d.cond.L.Unlock()
			d.cond.Broadcast()
			go d.connect(&u)
		}
	}
}

// connect to upstream server and keep tunnel alive
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

					d.cond.L.Lock()
					// set alive only when received pong
					u.alive = time.Now()
					d.cond.L.Unlock()
					d.cond.Broadcast()

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

// push packet to packet queue
func (d *Dialer) push(p *packet) {
	// TODO:
}

// check if there is any alive upstream
func (d *Dialer) check() (alive bool) {
	for _, v := range d.pool {
		if v.proto == "udp" {
			return true
		}
		if time.Now().Sub(v.alive) < d.keepalive {
			return true
		}
	}
	return false
}
