package trafcacc

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

func TestMain(tm *testing.M) {
	go func() {
		time.Sleep(time.Second * 5)
		panic("case test took too long")
	}()

	os.Exit(tm.Run())
}

func testHandle(conn net.Conn) {

}

func TestDial(t *testing.T) {
	HandleFunc("tcp://:51010-51020", testHandle)

	d := NewDialer()
	d.Setup("tcp://127.0.0.1:51010-51020")

	conn, err := d.Dial()
	if err != nil {
		fmt.Println(err)
		t.Fail()
		return
	}

	conn.Write([]byte("GET\n\n"))

	conn.Close()

}
