package main

import (
	"encoding/json"
	"fmt"
	"sync"

	mlib "github.com/wsva/monitor_lib_go"
)

func main() {
	if err := initGlobals(); err != nil {
		fmt.Println(err)
		return
	}

	wg := &sync.WaitGroup{}
	for _, v := range targetList {
		go checkOracle(v, wg)
		wg.Add(1)
	}
	wg.Wait()

	jsonBytes, _ := json.Marshal(resultsRuntime)
	fmt.Println(mlib.MessageTypeMRList + string(jsonBytes))
}

func checkOracle(t TargetOracle, wg *sync.WaitGroup) {
	defer wg.Done()

	if !t.Enable {
		return
	}

	var mdinfo MDInfo
	defer func() {
		jsonString, err := mdinfo.MD.JSONString()
		resultsRuntimeLock.Lock()
		if err != nil {
			resultsRuntime = append(resultsRuntime,
				mlib.GetMR(t.Name, t.Address, mlib.MTypeOracle, "", err.Error()))
		} else {
			resultsRuntime = append(resultsRuntime,
				mlib.GetMR(t.Name, t.Address, mlib.MTypeOracle, jsonString, ""))
		}
		resultsRuntimeLock.Unlock()
	}()

	cr, err := checkConnectivity(&t.DB)
	if err != nil {
		mdinfo.AddError(fmt.Sprintf("CheckConnectivity error: %v", err))
		return
	} else {
		mdinfo.Lock.Lock()
		mdinfo.MD.ConnectivityOK = cr
		mdinfo.Lock.Unlock()
	}

	defer t.DB.Close()
	mdinfo.WG.Add(5)
	go checkArchiveLog(&t.DB, &mdinfo)
	go checkASM(&t.DB, &mdinfo)
	go checkTableSpace(&t.DB, t.ExcludeTableSpace, &mdinfo)
	go checkTableLock(&t.DB, &mdinfo)
	go checkPassword(&t.DB, &mdinfo)
	mdinfo.WG.Wait()
}
