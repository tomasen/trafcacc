package trafcacc

import (
	"encoding/gob"
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
	fmt.Println("server0")
	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	in := 1

	for {
		fmt.Println("server1")
		err := dec.Decode(&in)
		if err != nil {
			fmt.Println("server read:", err)
			break
		}
		in *= 2
		fmt.Println("server2")
		err = enc.Encode(in)
		if err != nil {
			fmt.Println("dialer write:", err)
			break
		}
	}
}

func TestDial(t *testing.T) {
	HandleFunc("tcp://:51010-51020", testHandle)

	d := NewDialer()
	d.Setup("tcp://127.0.0.1:51010-51020")

	fmt.Println("Dail0")
	conn, err := d.Dial()
	if err != nil {
		fmt.Println(err)
		t.Fail()
		return
	}
	fmt.Println("Dail1")
	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	out, in := 1, 1

	for {
		fmt.Println("Dail3")
		err := enc.Encode(in)
		if err != nil {
			fmt.Println("dialer write:", err)
			t.Fail()
			break
		}
		fmt.Println("Dail4")
		err = dec.Decode(&out)
		if err != nil {
			fmt.Println("dialer read:", err)
			t.Fail()
			break
		}
		fmt.Print(out, " ")
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
