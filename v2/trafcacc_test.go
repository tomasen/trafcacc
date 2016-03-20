package trafcacc

import (
	"encoding/gob"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
)

func TestMain(tm *testing.M) {
	if len(os.Getenv("IPERF")) <= 0 {
		go func() {
			time.Sleep(time.Second * 8)
			panic("case test took too long")
		}()
	}
	os.Exit(tm.Run())
}

func testDialServe0(conn net.Conn) {

	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	in := 1

	for {
		err := dec.Decode(&in)
		if err != nil {
			logrus.Warnln("server read error", err)
			break
		}
		in++
		err = enc.Encode(in)
		if err != nil {
			logrus.Fatalln("server write error", err)
			break
		}
	}
}

func TestDial(t *testing.T) {
	srv := NewServeMux()
	srv.HandleFunc("tcp://:51010-51020", testDialServe0)

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
			logrus.Warnln("dialer read error", err)
			t.Fail()
			break
		}

		if out != in+1 {
			t.Fail()
			break
		}

		in = out + 1
		if out > 2000 {
			break
		}
	}
	conn.Close()
}

func TestHTTP(t *testing.T) {

	Accelerate("tcp://:41601-41604", "tcp://bing.com:80", BACKEND)
	Accelerate("tcp://:50580", "tcp://127.0.0.1:41601-41604", FRONTEND)

	client := &http.Client{}
	req, _ := http.NewRequest("GET", "http://127.0.0.1:50580/robots.txt", nil)
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
}

func TestIPERF(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		<-c
		panic(nil)
	}()

	if len(os.Getenv("IPERF")) != 0 {
		cmd := exec.Command("iperf3", "-s", "", "-p", "5203")
		go iperfExec(cmd)

		Accelerate("tcp://:41501-41504", "tcp://127.0.0.1:5203", BACKEND)
		//Accelerate("tcp://:41501-41504", "tcp://54.222.184.194:5201", BACKEND)
		time.Sleep(time.Second)
		Accelerate("tcp://:50500", "tcp://127.0.0.1:41501-41504", FRONTEND)
		time.Sleep(time.Second)

		//iperfExec(exec.Command("iperf3", "-c", "127.0.0.1", "-p", "50500", "-R", "-P", "3"))
		//iperfExec(exec.Command("iperf3", "-c", "127.0.0.1", "-p", "50500", "-b", "10M"))
		iperfExec(exec.Command("iperf3", "-c", "127.0.0.1", "-p", "50500"))
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, 15) // note the minus sign
		}
	}
}

func iperfExec(cmd *exec.Cmd) {
	logrus.Debugln(cmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		logrus.Fatalln(err)
	}
	err = cmd.Wait()
}
