package trafcacc

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
)

var (
	_echoServerAddr = "127.0.0.1:62863"
)

func TestMain(m *testing.M) {
	// start echo server
	go servTCPEcho()

	go Accelerate("tcp://:51500", "tcp://127.0.0.1:51501-51504", false)

	go Accelerate("tcp://:51501-51504", "tcp://"+_echoServerAddr, true)
	// start tcp Accelerate front-end
	// start tcp Accelerate back-end
	// start tcp client
	// start udp Accelerate
	// start udp client
	rand.Seed(time.Now().UnixNano())
	time.Sleep(time.Second)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		<-c
		panic(nil)
	}()

	go func() {
		time.Sleep(time.Second * 8)
		panic("case test took too long")
	}()

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
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			testEchoConn(t)
			wg.Done()
		}()
	}
	wg.Wait()
}

func testEchoConn(t *testing.T) {
	conn, err := dialTimeout("tcp", "127.0.0.1:51500", time.Second*time.Duration(_BackendDialTimeout))
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	for i := 0; i < 10; i++ {
		testEchoRound(conn, t)
	}

	time.Sleep(time.Second)
}

func testEchoRound(conn net.Conn, t *testing.T) {
	conn.SetDeadline(time.Now().Add(time.Second * 10))

	n := rand.Int()%(buffersize*10) + 10
	out := randomBytes(n)
	n0, err := conn.Write(out)
	if err != nil {
		fmt.Println(err)
		t.Fail()
	}

	rcv := make([]byte, n)
	n1, err := io.ReadFull(conn, rcv)
	if err != nil && err != io.EOF {
		fmt.Println(err)
		t.Fail()
	}
	if !bytes.Equal(out[:n0], rcv[:n1]) {
		fmt.Println("out: ", n0, "in:", n1)

		fmt.Println("out:", hex.EncodeToString(out))
		fmt.Println("in: ", hex.EncodeToString(rcv))
		fmt.Println(errors.New("echo server reply is not match"))
		t.Fail()
	} else {
		fmt.Println("echo test", n0, "pass")
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

func TestGoroutineLeak(t *testing.T) {
	n := runtime.NumGoroutine()
	log.Println("NumGoroutine:", n)
	if n > 5 {
		t.Fail()
		panic("goroutine leak")
	}
}
