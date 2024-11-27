package main

import (
	"time"

	"chidweb/pkg/server"
)

func main() {
	ser := server.NewServer("8080")
	ser.Start()
	time.Sleep(1 * time.Second)
	ser.Wait()

}
