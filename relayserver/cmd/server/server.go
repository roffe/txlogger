package main

import (
	"log"

	"github.com/roffe/txlogger/relayserver"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	server := relayserver.New()
	if err := server.Run(":9000"); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
