package trafcacc

import (
	"encoding/gob"
	"errors"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
)

type dialer struct {
	keepalive time.Duration

	pool []*upstream
	cond *sync.Cond

	identity uint32
	atomicid uint32

	packetQueue packetQueue
	closedQueue closedQueue
}

// NewDialer TODO: comment
func NewDialer() Dialer {
	return &dialer{
		cond:      sync.NewCond(&sync.Mutex{}),
		keepalive: time.Second * 3,
		identity:  rand.Uint32(),
	}
}

// Setup upstream servers
func (d *dialer) Setup(server string) {
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

// Dial acts like net.Dial
func (d *dialer) Dial() (net.Conn, error) {
	return d.DialTimeout(time.Duration(0))
}

// DialTimeout is the maximum amount of time a dial will wait for
// a connect to complete. If Deadline is also set, it may fail
// earlier.
//
// The default is 0 means no timeout.
//
func (d *dialer) DialTimeout(timeout time.Duration) (net.Conn, error) {
	// wait for upstream online and alive
	ch := make(chan struct{}, 1)
	go func() {
		d.cond.L.Lock()
		for !d.check() {
			d.cond.Wait()
		}
		d.cond.L.Unlock()

		ch <- struct{}{}
	}()
	if timeout == time.Duration(0) {
		<-ch
	} else {
		select {
		case <-ch:
		case <-time.After(timeout):
			return nil, errors.New("i/o timeout")
		}
	}

	conn := &dialerConn{
		dialer: d,
		connid: atomic.AddUint32(&d.atomicid, 1),
	}

	// send connect cmd
	d.write(&packet{
		Connid: conn.connid,
		Cmd:    connect,
	})

	return conn, nil
}

func (d *dialer) write(p *packet) error {
	p.Senderid = d.identity
	successed := false

	// pick upstream tunnel and send packet
	for _, u := range d.pickupstreams() {
		err := u.encoder.Encode(&p)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Warnln("Dialer encode packet to upstream errror")
		} else {
			successed = true
		}
	}
	if successed {
		// return error if all failed
		return nil
	}
	return errors.New("dialer encoder error")
}

// connect to upstream server and keep tunnel alive
func (d *dialer) connect(u *upstream) {
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

func (d *dialer) pickupstreams() []*upstream {
	// avoid duplicate
	length := len(d.pool)
	switch length {
	case 0:
		return nil
	case 1:
		return d.pool
	default:
		idx := rand.Intn(length)
		return []*upstream{d.pool[idx], d.pool[(idx+1)%length]}
	}
}

// push packet to packet queue
func (d *dialer) push(p *packet) {
	// TODO:

	switch p.Cmd {

	case connected:
		if _, ok := d.packetQueue[p.Senderid][p.Connid]; !ok {
			delete(d.closedQueue[p.Senderid], p.Connid)
		}

	case close:
		delete(d.packetQueue[p.Senderid], p.Connid)
		d.closedQueue[p.Senderid][p.Connid] = struct{}{}

	default: //data
		if _, ok := d.closedQueue[p.Senderid][p.Connid]; !ok {
			if _, ok := d.packetQueue[p.Senderid][p.Connid][p.Seqid]; !ok {
				d.packetQueue[p.Senderid][p.Connid][p.Seqid] = p
			} // else drop duplicated packet
		} // else drop closed packet
	}
}

// check if there is any alive upstream
func (d *dialer) check() (alive bool) {
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
