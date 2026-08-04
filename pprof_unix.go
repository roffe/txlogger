//go:build pprof && !windows

package main

import (
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
}
