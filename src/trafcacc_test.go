package trafcacc

import (
	"fmt"
	"testing"
)

//func TestMain(m *testing.M) {
// start echo server
// start tcp Accelerate front-end
// start tcp Accelerate back-end
// start tcp client
// start udp Accelerate
// start udp client
//}

func TestOverflow(t *testing.T) {
	for i := uint64(0); ; i++ {
		i--
		fmt.Println(i)
		i++
		fmt.Println(i)
		break
	}
}
