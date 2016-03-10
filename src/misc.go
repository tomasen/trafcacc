package trafcacc

import (
	"net"
	"strings"
	"time"
)

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
