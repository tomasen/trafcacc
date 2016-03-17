package trafcacc

type cmd uint8

const (
	data cmd = iota
	close
	connect
	ping
	pong
)

type packet struct {
	Serverid uint32
	Connid   uint32
	Seqid    uint32
	Buf      []byte
	Cmd      cmd
}
