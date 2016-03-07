package trafcacc

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"testing"
	"time"
)

var (
	_echoServerAddr = "127.0.0.1:62863"
)

func TestMain(m *testing.M) {
	// start echo server
	go servTCPEcho()

	go Accelerate("tcp://:51500", "tcp://127.0.0.1:51501-51510", false)

	go Accelerate("tcp://:51501-51510", "tcp://"+_echoServerAddr, true)
	// start tcp Accelerate front-end
	// start tcp Accelerate back-end
	// start tcp client
	// start udp Accelerate
	// start udp client
	rand.Seed(time.Now().UnixNano())
	time.Sleep(time.Second)
	os.Exit(m.Run())
}

func servTCPEcho() {
	l, err := net.Listen("tcp", _echoServerAddr)
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		os.Exit(1)
	}
	// Close the listener when the application closes.
	defer l.Close()
	fmt.Println("Listening on " + _echoServerAddr)
	for {
		// Listen for an incoming connection.
		c, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			os.Exit(1)
		}
		// Handle connections in a new goroutine.
		go func(c net.Conn) {
			defer c.Close()

			_, err := io.Copy(c, c)
			switch err {
			case io.EOF:
				err = nil
				return
			case nil:
				return
			}
			panic(err)
		}(c)
	}
}

// TestEchoServer ---
func TestEchoServer(t *testing.T) {
	conn, err := dialTimeout("tcp", "127.0.0.1:51500", time.Second*time.Duration(_BackendDialTimeout))
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	n := rand.Int() % 10
	for i := 0; i < n; i++ {
		testEchoRound(conn)
	}
}

func testEchoRound(conn net.Conn) {
	conn.SetDeadline(time.Now().Add(time.Second * 10))

	n := rand.Int()%2048 + 10
	out := randomBytes(n)
	n0, err := conn.Write(out)
	if err != nil {
		panic(err)
	}

	rcv := make([]byte, n)
	n1, err := io.ReadFull(conn, rcv)
	if err != nil && err != io.EOF {
		panic(err)
	}
	if !bytes.Equal(out[:n0], rcv[:n1]) {
		fmt.Println("out: ", n0, "in:", n1)

		fmt.Println("out: ", hex.EncodeToString(out), "in:", hex.EncodeToString(rcv))
		panic(errors.New("echo server reply is not match"))
	}
}

func randomBytes(n int) []byte {

	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i := 0; i < n; i++ {
		b[i] = byte(rand.Int())
	}

	return b
}
