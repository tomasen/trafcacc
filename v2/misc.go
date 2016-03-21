package trafcacc

import (
	"net"
	"runtime"
	"time"

	"github.com/Sirupsen/logrus"
)

func (t *trafcacc) Status() {
	// print status
	s := new(runtime.MemStats)

	fields := logrus.Fields{
		"NumGoroutine": runtime.NumGoroutine(),
		"Alloc":        s.Alloc,
		"HeapObjects":  s.HeapObjects,
	}

	logrus.WithFields(fields).Infoln(t.roleString(), "status")
}

func acceptTCP(ln net.Listener, f HandlerFunc) {
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
