package trafcacc

import (
	"encoding/binary"
	"net"
)

type packet struct {
	connid uint32
	seqid  uint32
	buf    []byte
}

var (
	atomicid uint32
)

// MARK: upstream
func sendpkt(p packet) {
	u := upool.next()
	if u.conn == nil {
		// dial
		switch u.proto {
		case "tcp":
			conn, err := net.Dial("tcp", u.addr)
			if err != nil {
				// TODO: reply error

				return
			}
			u.conn = conn
			// TODO: keep reading
		case "udp":
			// TODO
		}
	}

	buf := p.buf
	if !isbackend {
		// encapsule packet
		hdr := make([]byte, 10)
		binary.LittleEndian.PutUint32(hdr[:4], p.connid)
		binary.LittleEndian.PutUint32(hdr[4:8], p.seqid)
		binary.LittleEndian.PutUint16(hdr[8:10], uint16(len(p.buf)+8))
		buf = append(hdr, buf...)
	}

	_, err := u.conn.Write(buf)
	if err != nil {
		u.conn = nil
		// if p.reply != nil {
		// 	// reply error
		// 	p.reply <- packet{p.connid, p.seqid, nil, nil}
		// }
		return
	}
}
