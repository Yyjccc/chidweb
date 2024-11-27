package main

import (
	client2 "chidweb/pkg/client"
	"log"
)

func main() {

	client := client2.NewClient("http://127.0.0.1:8080", "172.19.173.38:8088", "yyjccc")
	if err := client.Start(); err != nil {
		log.Fatal(err)
	}
	client.Wait()
}
