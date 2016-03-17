package trafcacc

import (
	"fmt"
	"testing"
)

func TestDial(t *testing.T) {
	d := &Dialer{}
	d.Setup("tcp://www.google.com:80")
	conn, err := d.Dial()
	if err != nil {
		t.Fail()
	}
	conn.Write([]byte("GET\n\n"))

	b := make([]byte, 4096)
	n, err := conn.Read(b)
	if err != nil {
		t.Fail()
	}

	fmt.Println(string(b[:n]))
}
