package trafcacc

import (
	"reflect"
	"testing"
	"time"
)

func TestPacket(t *testing.T) {
	now := time.Now().UnixNano()
	p0 := &packet{1, 2, 3, []byte("12"), close, false, now}
	udpbuf := make([]byte, buffersize)

	n := p0.encode(udpbuf)
	if n != 7 {
		t.Fail()
	}
	p1 := &packet{}
	p1.Time = now
	err := decodePacket(udpbuf[:n], p1)
	if err != nil || !reflect.DeepEqual(p1, p0) {
		t.Fail()
	}
}
