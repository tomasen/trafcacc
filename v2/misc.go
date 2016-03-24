package trafcacc

import (
	"net"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dustin/go-humanize"
)

var (
	udpBufferPool = &sync.Pool{New: func() interface{} { return make([]byte, buffersize) }}
	waitGroupPool = &sync.Pool{New: func() interface{} { return new(sync.WaitGroup) }}
)

func (t *trafcacc) Status() {
	// print status
	s := new(runtime.MemStats)

	runtime.ReadMemStats(s)

	fields := logrus.Fields{
		"NumGoroutine": runtime.NumGoroutine(),
		"Alloc":        humanize.Bytes(s.Alloc),
		"HeapObjects":  s.HeapObjects,
	}

	if logrus.GetLevel() >= logrus.DebugLevel {
		t.pool.mux.RLock()
		var us, ts, ur, tr string
		var su, st, ru, rt uint64
		for _, v := range t.pool.pool {
			s := atomic.LoadUint64(&v.sent)
			r := atomic.LoadUint64(&v.recv)
			if v.proto == udp {
				su += s
				ru += r
				us += humanbyte(s) + ","
				ur += humanbyte(r) + ","
			} else {
				st += s
				rt += r
				ts += humanbyte(s) + ","
				tr += humanbyte(r) + ","
			}
		}
		t.pool.mux.RUnlock()
		fields["Sent(U)"] = humanbyte(su) + "(" + strings.TrimRight(us, ",") + ")"
		fields["Recv(U)"] = humanbyte(ru) + "(" + strings.TrimRight(ur, ",") + ")"

		fields["Sent(T)"] = humanbyte(st) + "(" + strings.TrimRight(ts, ",") + ")"
		fields["Recv(T)"] = humanbyte(rt) + "(" + strings.TrimRight(tr, ",") + ")"
	}

	logrus.WithFields(fields).Infoln(t.roleString(), "status")
}

func humanbyte(n uint64) string {
	return strings.Replace(humanize.Bytes(n), " ", "", 1)
}

func acceptTCP(ln net.Listener, f func(net.Conn)) {
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

		go f(conn)
	}
}
