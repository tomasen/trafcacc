package trafcacc

import (
	"fmt"
	"testing"
)

//func TestMain(m *testing.M) {
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
