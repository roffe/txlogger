package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/roffe/txlogger/pkg/server"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	s, err := server.New()
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	ss := <-sig
	log.Println("Received signal:", ss)
}
