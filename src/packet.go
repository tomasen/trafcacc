package trafcacc

import (
	"encoding/binary"
	"net"
)

type packet struct {
	connid uint32
	seqid  uint32
	buf    []byte
	reply  chan packet
}

var (
	atomicid uint32
	pktqueue = make(chan packet, maxpacketqueue)
)

// send to upstream
func (s *serv) sendup(p packet) {
	select {
	case pktqueue <- p:
	default:
	}
}

// MARK: upstream
func (u *upstream) sendpkt(p packet) {
	// TODO: 是否建立新连接基于:
	// ??? 是否 buf 是 nil
	// 是否已经建立过连接，没有则必然建立
	// backend 是新的connid一定就是要建立新连接
	// 是 front 还是 backend
	// backend 的话，不是新的connid就使用上一个 connid 的连接

	if u.conns[p.connid] == nil {
		// dial
		switch u.proto {
		case "tcp":
			conn, err := net.Dial("tcp", u.addr)
			if err != nil {
				// reply error
				p.reply <- p
				return
			}
			u.conns[p.connid] = conn
			// TODO: keep reading
		case "udp":
			// TODO
		}
	}

	buf := p.buf
	if isfrontend {
		// encapsule packet
		hdr := make([]byte, 10)
		binary.LittleEndian.PutUint32(hdr[:4], p.connid)
		binary.LittleEndian.PutUint32(hdr[4:8], p.seqid)
		binary.LittleEndian.PutUint16(hdr[8:10], uint16(len(p.buf)+8))
		buf = append(hdr, buf...)
	}

	_, err := u.conns[p.connid].Write(buf)
	if err != nil {
		delete(u.conns, p.connid)
		if p.reply != nil {
			// reply error
			p.reply <- packet{p.connid, p.seqid, nil, nil}
		}
		return
	}
}
