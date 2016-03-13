// debug.go 辅助 Debug 的一些 helper function

package trafcacc

import (
	"sync"

	log "github.com/Sirupsen/logrus"
)

var (
	routineList = make(map[string]int)
	routineMux  = &sync.RWMutex{}
)

func routineAdd(name string) {
	routineMux.Lock()
	routineList[name] = routineList[name] + 1
	routineMux.Unlock()
}

func routineDel(name string) {
	routineMux.Lock()
	routineList[name] = routineList[name] - 1
	routineMux.Unlock()
}

func routinePrint() {
	routineMux.RLock()
	totalGoroutineTracked := 0
	for _, v := range routineList {
		totalGoroutineTracked += v
	}
	log.WithFields(log.Fields{
		"Total":  totalGoroutineTracked,
		"Detail": routineList,
	}).Infoln("live goroutine list")
	routineMux.RUnlock()
}

func keysOfmap(m map[uint32]*packet) []uint32 {
	rlen := len(m)
	if rlen > 10 {
		rlen = 10
	}
	r := make([]uint32, rlen)
	i := 0
	for k := range m {
		r[i] = k
		i++
		if i >= rlen {
			break
		}
	}
	return r
}

func shrinkString(s string) string {
	l := len(s)
	if l > 20 {
		return s[:10] + "..." + s[l-10:l]
	}
	return s
}
