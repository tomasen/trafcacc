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
	if rlen > 15 {
		rlen = 15
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
	if l > 30 {
		return s[:15] + "..." + s[l-15:l]
	}
	return s
}
