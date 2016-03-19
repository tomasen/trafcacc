package trafcacc

import (
	"encoding/gob"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
)

func TestMain(tm *testing.M) {
	go func() {
		time.Sleep(time.Second * 5)
		panic("case test took too long")
	}()

	os.Exit(tm.Run())
}

func testHandle(conn net.Conn) {

	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	in := 1

	for {

		err := dec.Decode(&in)
		if err != nil {
			logrus.Fatalln("server read error", err)
			break
		}
		in *= 2

		err = enc.Encode(in)
		if err != nil {
			logrus.Fatalln("server write error", err)
			break
		}
	}
}

func TestDial(t *testing.T) {
	HandleFunc("tcp://:51010-51020", testHandle)

	d := NewDialer()
	d.Setup("tcp://127.0.0.1:51010-51020")

	conn, err := d.Dial()
	if err != nil {
		logrus.Fatalln("dialer dial error", err)
		t.Fail()
		return
	}

	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	out, in := 1, 1

	for {

		err := enc.Encode(in)
		if err != nil {
			logrus.Fatalln("dialer write error", err)
			t.Fail()
			break
		}

		err = dec.Decode(&out)
		if err != nil {
			logrus.Fatalln("dialer read error", err)
			t.Fail()
			break
		}
		fmt.Print(in, " ", out, " ")
		if out != in*2 {
			t.Fail()
			break
		}

		in = out * 2
		if out > 10240 {
			break
		}
	}
	conn.Close()
}
