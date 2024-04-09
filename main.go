package main

import (
	"fmt"
	"graduation_project/internal/routers"
)

const APIPort = 8080

func main() {
	r := routers.SetupRouter()
	if err := r.Run(fmt.Sprintf(":%d", APIPort)); err != nil {
		panic(fmt.Sprintf("run server failed, err:%v\n", err))
	}
	//pprof.Register(r)

}
