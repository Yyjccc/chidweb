package main

import (
	client2 "chidweb/pkg/client"
	"chidweb/pkg/common"
	"flag"
	"os"
)

func main() {

	serverAddr := flag.String("s", "", "server url")
	targetAddr := flag.String("t", "", "target tcp address")
	_ = flag.String("l", "info", "log level")
	flag.Parse()
	if *serverAddr == "" {
		common.Error("server url is required.")
		os.Exit(1)
	}
	if *targetAddr == "" {
		common.Error("target tcp address is required.")
		os.Exit(1)
	}
	clientID := common.GenerateName(7)
	cli := client2.NewClient(*serverAddr, *targetAddr, clientID)
	if err := cli.Start(); err != nil {
		common.Fatal(err.Error())
	}
	cli.Wait()
}
