package trafcacc

import (
	"sync"

	log "github.com/Sirupsen/logrus"
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

func routineGet(name string) int32 {
	routineMux.Lock()
	defer routineMux.Unlock()
	return routineList[name]
}

func routinePrint() {
	routineMux.RLock()
	log.Infoln(routineList)
	routineMux.RUnlock()
}
