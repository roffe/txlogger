//go:build pprof

package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
)

func init() {
	// kill -USR1 <pid> to dump leaks
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGUSR1)
	go func() {
		for range sig {
			pprof.Lookup("goroutineleak").WriteTo(os.Stdout, 1)
		}
	}()

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
}
