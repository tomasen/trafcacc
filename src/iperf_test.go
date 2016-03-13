package trafcacc

import (
	"os"
	"os/exec"
	"syscall"
	"testing"

	log "github.com/Sirupsen/logrus"
)

func TestIperfTCP(t *testing.T) {
	// log.SetLevel(log.DebugLevel)

	if len(os.Getenv("IPERF")) != 0 {
		cmd := exec.Command("iperf3", "-s", "", "-p", "5203")
		go iperf_exec(cmd)

		frontend = Accelerate("tcp://:50500", "tcp://127.0.0.1:41501-41504", FRONTEND)
		backend = Accelerate("tcp://:41501-41504", "tcp://127.0.0.1:5203", BACKEND)

		iperf_exec(exec.Command("iperf3", "-c", "127.0.0.1", "-p", "50500", "-R", "-P", "3"))
		iperf_exec(exec.Command("iperf3", "-c", "127.0.0.1", "-p", "50500"))

		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, 15) // note the minus sign
		}
	}
}

func iperf_exec(cmd *exec.Cmd) {

	log.Debugln(cmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		log.Fatalln(err)
	}
	err = cmd.Wait()
}
