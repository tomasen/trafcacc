package trafcacc

import (
	"log"
	"net"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const maxopenfile = 3267600

func dialTimeout(network, address string, timeout time.Duration) (conn net.Conn, err error) {
	m := int(timeout / time.Second)
	for i := 0; i < m; i++ {
		conn, err = net.DialTimeout(network, address, timeout)
		if err == nil || !strings.Contains(err.Error(), "can't assign requested address") {
			break
		}
		time.Sleep(time.Second)
	}
	return
}

func keysOfmap(m map[uint32]*packet) (r []uint32) {
	for k := range m {
		r = append(r, k)
	}
	return
}

func shrinkString(s string) string {
	l := len(s)
	if l > 30 {
		return s[:15] + "..." + s[l-15:l]
	}
	return s
}

func incMaxopenfile() {

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
}

func incGomaxprocs() {
	cpu := runtime.NumCPU()
	if cpu > runtime.GOMAXPROCS(-1) {
		runtime.GOMAXPROCS(cpu)
	}
}
