package main

import (
	"chidweb/pkg/common"
	"chidweb/pkg/server"
	"flag"
	"fmt"
	"strings"
)

func main() {

	tcpPort := flag.String("p", "0", "tcp listener port ")
	httpPort := flag.Int("s", 8080, "http listener port")
	flag.Parse()
	fmt.Println(*tcpPort)
	ports := strings.Split(*tcpPort, ",")
	_ = flag.String("l", "info", "log level")
	ser := server.NewServer(*httpPort, ports)
	err := ser.Start()
	if err != nil {
		common.Error("%v", err)
	}
	ser.Wait()

}
