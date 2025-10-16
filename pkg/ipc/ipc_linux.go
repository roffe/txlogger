package ipc

import (
	"log"
	"net"
	"os"
	"path/filepath"
)

var socketFile = filepath.Join(os.TempDir(), "txlogger.sock")

func IsRunning() bool {
	if fileExists(socketFile) {
		if !ping() {
			log.Println("txlogger is not running, removing stale socket file")
			if err := os.Remove(socketFile); err != nil {
				log.Printf("failed to remove stale socket file: %v", err)
			}
		} else {
			log.Println("txlogger is running, sending show request over socket")
			sendShow()
			return true
		}
	}
	return false
}

func dial() (net.Conn, error) {
	return net.Dial("unix", socketFile)
}

func listen() (net.Listener, error) {
	return net.Listen("unix", socketFile)
}
