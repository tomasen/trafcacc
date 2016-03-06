package main

import (
	"flag"
	"log"
	"runtime"
	"syscall"

	"github.com/tomasen/trafcacc/src"
)

const maxopenfile = 3267600

func main() {

	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		log.Fatal("failed to get NOFILE rlimit: ", err)
	}

	if lim.Cur < maxopenfile || lim.Max < maxopenfile {
		if lim.Cur < maxopenfile {
			lim.Cur = maxopenfile
		}
		if lim.Max < maxopenfile {
			lim.Max = maxopenfile
		}

		if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
			log.Fatal("failed to set NOFILE rlimit: ", err)
		}
	}

	runtime.GOMAXPROCS(runtime.NumCPU())

	// -listen=tcp://:500 -upstream=udp://172.0.0.1:2000-2100
	// -listen=udp://:2000-2100 -upstream=tcp://172.0.0.1:500
	listen := flag.String("listen", "<proto>://<ip>:<port begin-end>[,...] eg. udp://0.0.0.0:500", "listen to")
	upstream := flag.String("upstream", "<proto>://<ip>:<port begin-end>[,...] eg. udp://172.0.0.1:2000-2100,192.168.1.1:2000-2050", "send to")
	backend := flag.Bool("backend", false, "work as backend")

	flag.Parse()

	trafcacc.Accelerate(*listen, *upstream, *backend)
}
