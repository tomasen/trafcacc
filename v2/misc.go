package trafcacc

import (
	"runtime"

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
