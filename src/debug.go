package trafcacc

import (
	log "github.com/Sirupsen/logrus"
	"sync"
)

var (
	routineList = make(map[string]int32)
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
	log.Debugln(routineList)
	routineMux.RUnlock()
}
