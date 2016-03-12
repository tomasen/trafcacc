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
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	log "github.com/Sirupsen/logrus"
)

var (
	_echoServerAddr               = "127.0.0.1:62863"
	dialRound                     = 1
	parrelelConn                  = 1
	echoRound                     = 2
	testTimeout     time.Duration = 15
)

var m = make(map[uint32]*packet)

func TestMain(tm *testing.M) {

	// log.SetLevel(log.DebugLevel)

	// start echo server
	go servTCPEcho()

	Accelerate("tcp://:51500", "tcp://127.0.0.1:51501-51504", FRONTEND)

	Accelerate("tcp://:51501-51504", "tcp://"+_echoServerAddr, BACKEND)
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
		const rname = "waitSignal"
		routineAdd(rname)
		defer routineDel(rname)

		<-c
		panic(nil)
	}()

	go func() {
		const rname = "testTimeout"
		routineAdd(rname)
		defer routineDel(rname)

		time.Sleep(time.Second * testTimeout)
		routinePrint()
		time.Sleep(time.Second)
		panic("RACE case test took too long")
	}()

	for i := uint32(0); i < 1000; i++ {
		m[i] = nil
	}

	os.Exit(tm.Run())
}

func servTCPEcho() {
	const rname = "servTCPEcho"
	routineAdd(rname)
	defer routineDel(rname)

	l, err := net.Listen("tcp", _echoServerAddr)
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		os.Exit(1)
	}
	// Close the listener when the application closes.
	defer l.Close()
	log.Debugln("Listening on " + _echoServerAddr)
	for {
		// Listen for an incoming connection.
		c, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			os.Exit(1)
		}
		// Handle connections in a new goroutine.
		go func(c net.Conn) {
			const rname = "servTCPEchoConn"
			routineAdd(rname)
			defer routineDel(rname)

			defer c.Close()

			for {
				c.SetReadDeadline(time.Now().Add(time.Second))
				_, err := io.Copy(c, c)

				switch err {
				case io.EOF:
					err = nil
					return
				case nil:
					return
				}
				if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
					// TODO: is this the only way to exit quickely when client close connection
					return
				}
				panic(err)
			}
		}(c)
	}
}

// TestEchoServer ---
func TestEchoServer(t *testing.T) {
	var wg sync.WaitGroup
	for j := 0; j < dialRound; j++ {
		for i := 0; i < parrelelConn; i++ {
			wg.Add(1)
			go func() {
				const rname = "testEchoConn"
				routineAdd(rname)
				defer routineDel(rname)
				defer wg.Done()

				testEchoConn(t)
			}()
		}
	}
	wg.Wait()
}

func testEchoConn(t *testing.T) {
	conn, err := dialTimeout("tcp", "127.0.0.1:51500", time.Second*time.Duration(_BackendDialTimeout))
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	for i := 0; i < echoRound; i++ {
		testEchoRound(conn, t)
	}

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
		log.WithFields(log.Fields{
			"len(out)": n0,
			"len(in)":  n1,
			"out":      shrinkString(hex.EncodeToString(out)),
			"in":       shrinkString(hex.EncodeToString(rcv)),
		}).Debugln(errors.New("echo server reply is not match"))
		t.Fail()
	} else {
		log.Debugln("echo test", n0, "pass")
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
	time.Sleep(time.Second)

	n := runtime.NumGoroutine()
	log.Infoln("NumGoroutine:", n)
	if n > 30 {
		routinePrint()
		//t.Fail()
		//panic("goroutine leak")
	}

	time.Sleep(time.Second)
}

func BenchmarkKeysOfmap(b *testing.B) {
	for i := 0; i < b.N; i++ {
		keysOfmap(m)
	}
}

func BenchmarkOldKeysOfmap(b *testing.B) {
	for i := 0; i < b.N; i++ {
		oldKeysOfmap(m)
	}
}

func oldKeysOfmap(m map[uint32]*packet) (r []uint32) {
	for k := range m {
		r = append(r, k)
	}
	return
}
