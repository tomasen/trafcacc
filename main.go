package main

import (
	"flag"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/tomasen/trafcacc/src"

	"net/http"
	_ "net/http/pprof"

	log "github.com/Sirupsen/logrus"
)

func main() {

	// -listen=tcp://:500 -upstream=udp://172.0.0.1:2000-2100
	// -listen=udp://:2000-2100 -upstream=tcp://172.0.0.1:500
	listen := flag.String("listen", "<proto>://<ip>:<port begin-end>[,...] eg. udp://0.0.0.0:500", "listen to")
	upstream := flag.String("upstream", "<proto>://<ip>:<port begin-end>[,...] eg. udp://172.0.0.1:2000-2100,192.168.1.1:2000-2050", "send to")
	backend := flag.Bool("backend", false, "work as backend")
	loglevel := flag.Bool("v", false, "set log level to debug")

	flag.Parse()

	if *loglevel {
		log.SetLevel(log.DebugLevel)
	}

	if *backend {
		trafcacc.Accelerate(*listen, *upstream, trafcacc.BACKEND)
	} else {
		trafcacc.Accelerate(*listen, *upstream, trafcacc.FRONTEND)
	}

	go func() {
		log.Println(http.ListenAndServe("localhost:60060", nil))
	}()

	go func() {
		s := new(runtime.MemStats)
		for {
			runtime.ReadMemStats(s)
			log.WithFields(log.Fields{
				"NumGoroutine": runtime.NumGoroutine(),
				"Alloc":        s.Alloc,
				"HeapAlloc":    s.HeapAlloc,
				"HeapIdle":     s.HeapIdle,
				"HeapInuse":    s.HeapInuse,
				"HeapObjects":  s.HeapObjects,
			}).Infoln("status")
			time.Sleep(time.Second)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)

	<-c
	// cleanup
	os.Exit(1)
}
