package trafcacc

import (
	"net"
	"runtime"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dustin/go-humanize"
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
		ustat := ""
		for _, v := range t.pool.pool {
			ustat += "S(" + humanize.Bytes(v.sent) + ")R(" + humanize.Bytes(v.recv) + ")"
		}
		t.pool.mux.RUnlock()
		fields["UP"] = ustat
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
