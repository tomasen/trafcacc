package trafcacc

import (
	"log"
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
	log.Println(routineList)
	routineMux.RUnlock()
}
