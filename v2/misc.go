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
		var up, down string
		for _, v := range t.pool.pool {
			up += "(" + strings.Replace(humanize.Bytes(atomic.LoadUint64(&v.sent)), " ", "", -1) + ")"
			down += "(" + strings.Replace(humanize.Bytes(atomic.LoadUint64(&v.recv)), " ", "", -1) + ")"
		}
		t.pool.mux.RUnlock()
		fields["UP"] = up
		fields["DOWN"] = down
	}

	logrus.WithFields(fields).Infoln(t.roleString(), "status")
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
