package trafcacc

import (
	"encoding/gob"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	//_ "net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"runtime/pprof"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
)

type test struct {
	N   int
	Buf []byte
}

func TestMain(tm *testing.M) {
	if len(os.Getenv("IPERF")) <= 0 {
		go func() {
			time.Sleep(time.Second * 15)
			panic("case test took too long")
		}()
	} else {
		// go func() {
		// 	logrus.Println(http.ListenAndServe(":60063", nil))
		// }()
	}
	os.Exit(tm.Run())
}

func testDialServe0(conn net.Conn) {

	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	in := test{}

	for {
		err := dec.Decode(&in)
		if err != nil {
			logrus.Warnln("server read error", err)
			break
		}
		in.N++
		err = enc.Encode(in)
		if err != nil {
			logrus.Fatalln("server write error", err)
			break
		}
	}
}

func TestDialTCP(t *testing.T) {
	testDial("tcp://127.0.0.1:51010-51020", "tcp://:51010-51020", t)
}

func TestDialUDP(t *testing.T) {
	testDial("udp://127.0.0.1:53010-53020", "udp://:53010-53020", t)
}

func testDial(f, s string, t *testing.T) {
	srv := NewServe()
	srv.HandleFunc(s, testDialServe0)

	d := NewDialer()
	d.Setup(f)

	conn, err := d.Dial()
	if err != nil {
		logrus.Fatalln("dialer dial error", err)
		t.Fail()
		return
	}

	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	in := test{N: 1}

	for {
		in.Buf = randomBytes(buffersize * 2)
		err := enc.Encode(in)
		if err != nil {
			logrus.Fatalln("dialer write error", err)
			t.Fail()
			break
		}
		out := test{}
		err = dec.Decode(&out)
		if err != nil {
			logrus.Warnln("dialer read error", err)
			t.Fail()
			break
		}

		if out.N != in.N+1 {
			t.Fail()
			break
		}

		if len(out.Buf) != len(in.Buf) {
			t.Fail()
			break
		}

		in.N = out.N + 1
		if out.N > 100 {
			break
		}
	}
	conn.Close()
}

func randomBytes(n int) []byte {

	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i := 0; i < n; i++ {
		b[i] = byte(rand.Int())
	}

	return b
}

func TestHTTPviaTCP(t *testing.T) {
	testHTTP("tcp://:41601-41604", "tcp://127.0.0.1:41601-41604", "50581", t)
}

func TestHTTPviaUDP(t *testing.T) {
	testHTTP("udp://:41701-41704", "udp://127.0.0.1:41701-41704", "50580", t)
}

func testHTTP(bc, fc, lport string, t *testing.T) {

	t0 := Accelerate(bc, "tcp://bing.com:80", BACKEND)
	t0.WaitforAlive()

	t1 := Accelerate("tcp://:"+lport, fc, FRONTEND)
	t1.WaitforAlive()

	client := &http.Client{}
	req, _ := http.NewRequest("GET", "http://127.0.0.1:"+lport+"/robots.txt", nil)
	req.Host = "bing.com"
	res, err := client.Do(req)
	if err != nil {
		t.Fail()
	}
	robots, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fail()
	}

	if !strings.Contains(string(robots), "Sitemap: http://www.bing.com/") {
		t.Fail()
	}

	t0.Status()
	t1.Status()
}

//
func BenchmarkPacketQueueAdd(b *testing.B) {
	var pqs = newPacketQueue()
	pqs.create(1, 1)
	pqs.create(1, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := &packet{uint32(1), uint32(1), uint32(i), nil, data, false}
		pqs.add(p)
		pqs.add(p)
	}
}

func TestIPERF(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		if <-c != syscall.SIGTERM {
			panic(nil)
		}
	}()

	if len(os.Getenv("IPERF")) != 0 {
		cmd0 := exec.Command("iperf3", "-s", "", "-p", "5203")
		cmd0.Start()
		pid := cmd0.Process.Pid
		go cmd0.Wait()

		t0 := Accelerate("tcp://:41501-41504,udp://:42401-42404", "tcp://127.0.0.1:5203", BACKEND)
		t0.WaitforAlive()
		t1 := Accelerate("tcp://:50500", "tcp://127.0.0.1:41501-41504,udp://127.0.0.1:42401-42404", FRONTEND)
		t1.WaitforAlive()

		//iperfExec(exec.Command("iperf3", "-c", "127.0.0.1", "-p", "50500", "-R", "-P", "3"))
		//iperfExec(exec.Command("iperf3", "-c", "127.0.0.1", "-p", "50500", "-b", "10M"))
		cmd1 := exec.Command("iperf3", "-c", "127.0.0.1", "-p", "50500", "--get-server-output")
		iperfExec(cmd1)
		if len(os.Getenv("CPU")) != 0 {
			f, err := os.Create("cpu.profile")
			if err != nil {
				log.Fatal(err)
			}
			pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
		}
		cmd1.Wait()

		cmd2 := exec.Command("iperf3", "-c", "127.0.0.1", "-p", "50500", "-R", "--get-server-output")
		iperfExec(cmd2)
		cmd2.Wait()

		t0.Status()
		t1.Status()

		pgid, err := syscall.Getpgid(pid)
		if err == nil {
			syscall.Kill(-pgid, 15) // note the minus sign
		}
	}
}

func iperfExec(cmd *exec.Cmd) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		logrus.Fatalln(err)
	}
}
