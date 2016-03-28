package trafcacc

import (
	"encoding/gob"
	"errors"
	"math/rand"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
)

type dialer struct {
	identity uint32
	atomicid uint32

	udpbuf []byte

	pool *streampool
	pqd  *packetQueue
}

func newDialer() *dialer {
	return &dialer{
		pool:     newStreamPool(),
		identity: rand.Uint32(),
		pqd:      newPacketQueue(),
	}
}

// Setup upstream servers
func (d *dialer) Setup(server string) {
	for _, e := range parse(server) {
		grp := 0
		for p := e.portBegin; p <= e.portEnd; p++ {
			u := upstream{proto: e.proto, addr: net.JoinHostPort(e.host, strconv.Itoa(p))}
			d.pool.append(&u, grp)
			go d.connect(&u)
		}
		grp++
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

	conn := newConn(d, d.identity, atomic.AddUint32(&d.atomicid, 1))

	d.pqd.create(conn.senderid, conn.connid)

	// send connect cmd
	d.write(&packet{
		Senderid: d.identity,
		Connid:   conn.connid,
		Cmd:      connect,
	})

	return conn, nil
}

func (d *dialer) streampool() *streampool {
	return d.pool
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
		go d.pingloop(u)

		atomic.StoreInt64(&u.alive, time.Now().UnixNano())
		d.pool.Broadcast()

		if u.proto == tcp {
			d.readtcp(u)
		} else {
			d.readudp(u)
		}

		u.close()
	}
}

func (d *dialer) readtcp(u *upstream) {
	for {
		p := packet{}
		err := u.decoder.Decode(&p)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error": err,
			}).Warnln("Dialer docode upstream packet")
			break
		}

		d.proc(u, &p)
	}
}

func (d *dialer) readudp(u *upstream) {
	for {
		p := packet{}
		udpbuf := make([]byte, buffersize)
		n, err := u.conn.Read(udpbuf)
		if err != nil {
			logrus.WithError(err).Warnln("dialer Read UDP error")
			break
		}
		if err := decodePacket(udpbuf[:n], &p); err != nil {
			logrus.WithError(err).Warnln("dialer gop decode from udp error")
			continue
		}
		p.udp = true

		d.proc(u, &p)
	}
}

func (d *dialer) proc(u *upstream, p *packet) {
	atomic.AddUint64(&u.recv, uint64(len(p.Buf)))

	switch p.Cmd {
	case pong:
		// set alive only when received pong
		atomic.StoreInt64(&u.alive, time.Now().UnixNano())
		d.pool.Broadcast()
	case ack:
		d.pool.cache.ack(p.Senderid, p.Connid, p.Seqid)
	case rqu:
		rp := d.pool.cache.get(p.Senderid, p.Connid, p.Seqid)
		if rp != nil {
			d.write(rp)
		}
	default:
		go d.push(p)
		go d.write(&packet{
			Senderid: p.Senderid,
			Connid:   p.Connid,
			Seqid:    p.Seqid,
			Cmd:      ack,
		})
	}
}

func (d *dialer) pingloop(u *upstream) {
	ch := time.Tick(time.Second)
	for {
		err := u.send(ping)
		if err != nil {
			u.close()
			break
		}
		<-ch
	}
}

func (d *dialer) write(p *packet) error {
	// return error if all failed
	return d.pool.write(p)
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
