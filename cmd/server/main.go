package main

import (
	"flag"
	"log"

	"chidweb/pkg/server"
)

func main() {
	httpPort := flag.String("http-port", "8080", "HTTP server port")
	flag.Parse()

	srv := server.NewServer(*httpPort)
	log.Printf("Starting server on port %s", *httpPort)
	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
