// flags.go 用来分析参数(例如命令行参数)的 parser

package trafcacc

import (
	"net"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
)

type endpoint struct {
	proto     string
	host      string
	portBegin int // port begin
	portEnd   int // port end
}

// 分析输入的控制参数
func parse(s string) (e []endpoint) {
	x := strings.Split(s, ",")
	if len(x) < 1 {
		log.Fatal("argument error", s)
	}

	for _, s0 := range x {
		e0 := endpoint{}
		x0 := strings.Split(s0, "://")
		if len(x0) < 2 {
			log.Fatal("argument error", s0)
		}
		switch x0[0] {
		case "tcp":
		case "udp":
		default:
			log.Fatal("unknown proto:", x0[0])
		}
		e0.proto = x0[0]
		h, p, err := net.SplitHostPort(x0[1])
		if err != nil {
			log.Fatal("argument", x0[1], "error:", err)
		}
		e0.host = h

		x1 := strings.Split(p, "-")
		if len(x1) < 1 {
			log.Fatal("argument port", p, "error:", err)
		}

		e0.portBegin, err = strconv.Atoi(x1[0])
		if err != nil {
			log.Fatal("argument port begin ", x1[0], "error:", err)
		}

		if len(x1) < 2 {
			e0.portEnd = e0.portBegin
		} else {
			e0.portEnd, err = strconv.Atoi(x1[1])
			if err != nil {
				log.Fatal("argument port end ", x1[1], "error:", err)
			}
		}

		e = append(e, e0)
	}
	return e
}
