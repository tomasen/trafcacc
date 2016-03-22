package trafcacc

import (
	"bytes"
	"encoding/gob"
	"errors"
	"io"
	"math/rand"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
)

// NewDialer TODO: comment
func NewDialer() Dialer {

	return &dialer{
		pool:     newStreamPool(),
		identity: rand.Uint32(),
		pqd:      newPacketQueue(),
	}
}

// Dialer TODO: comment
type Dialer interface {
	Setup(server string)
	Dial() (net.Conn, error)
	DialTimeout(timeout time.Duration) (net.Conn, error)
}

type dialer struct {
	pool *streampool

	identity uint32
	atomicid uint32

	pqd *packetQueue

	udpbuf []byte
}

// Setup upstream servers
func (d *dialer) Setup(server string) {
	for _, e := range parse(server) {
		for p := e.portBegin; p <= e.portEnd; p++ {
			u := upstream{proto: e.proto, addr: net.JoinHostPort(e.host, strconv.Itoa(p))}
			d.pool.append(&u)
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
		d.pool.waitforalive()

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

	conn := &packetconn{
		pconn:    d,
		senderid: d.identity,
		connid:   atomic.AddUint32(&d.atomicid, 1),
	}

	d.pqd.create(conn.senderid, conn.connid)

	// send connect cmd
	d.write(&packet{
		Senderid: d.identity,
		Connid: conn.connid,
		Cmd:    connect,
	})

	return conn, nil
}

func (d *dialer) pq() *packetQueue {
	return d.pqd
}

func (d *dialer) role() string {
	// TODO: return tag
	return "dialer"
}

// connect to upstream server and keep tunnel alive
func (d *dialer) connect(u *upstream) {
	for {
		conn, err := net.Dial(u.proto, u.addr)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"addr":  u.addr,
				"error": err,
			}).Warnln("Dialer dial upstream error")
			time.Sleep(time.Second)
			continue
		}

		u.conn = conn

		switch u.proto {
		case tcp:
			u.encoder = gob.NewEncoder(conn)
			u.decoder = gob.NewDecoder(conn)
		case udp:
		}

		// begin to ping
		err = u.send(ping)
		if err != nil {
			u.close()
			continue
		}

		d.readloop(u)

		u.close()
	}
}

func (d *dialer) readloop(u *upstream) {
	for {
		p := packet{}
		if u.proto == tcp {
			err := u.decoder.Decode(&p)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"error": err,
				}).Warnln("Dialer docode upstream packet")
				break
			}
		} else { //  if u.proto == udp
			udpbuf := udpBufferPool.Get().([]byte)
			defer udpBufferPool.Put(udpbuf)
			n, err := u.conn.Read(udpbuf)
			if err != nil {
				logrus.WithError(err).Warnln("dialer Read UDP error")
				break
			}
			if err := gob.NewDecoder(bytes.NewReader(udpbuf[:n])).Decode(&p); err != nil && err != io.EOF {
				logrus.WithError(err).Warnln("dialer gop decode from udp error")
				continue
			}
		}
		if p.Cmd == pong {
			// set alive only when received pong
			atomic.StoreInt64(&u.alive, time.Now().UnixNano())
			d.pool.Broadcast()

			go func() {
				<-time.After(time.Second)
				err := u.send(ping)
				if err != nil {
					// TODO: close connection?
				}
			}()
			continue
		}
		go d.push(&p)
	}
}

func (d *dialer) write(p *packet) error {
	p.Senderid = d.identity
	successed := false

	// pick upstream tunnel and send packet
	for _, u := range d.pool.pickupstreams() {
		err := u.sendpacket(p)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Warnln("Dialer encode packet to upstream errror")
		} else {
			successed = true
		}
	}
	if successed {
		return nil
	}

	// return error if all failed
	return errors.New("dialer encoder error")
}

// push packet to packet queue
func (d *dialer) push(p *packet) {

	switch p.Cmd {
	case connected:
		// TODO: maybe move d.pqd.create(p.Senderid, p.Connid) here?

	case data, closed: //data
		d.pqd.add(p)

	default:
		logrus.WithFields(logrus.Fields{
			"Cmd": p.Cmd,
		}).Warnln("unexpected Cmd in packet")
	}
}
