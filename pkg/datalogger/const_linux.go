//go:build linux
// +build linux

package datalogger

import (
	"log"
	"os"
)

const (
	LOGPATH = "logs/"
)

func init() {
	if err := os.MkdirAll(LOGPATH, os.ModePerm); err != nil {
		log.Println("Error creating log directory: ", err)
	}
	log.Println("LOGPATH: ", LOGPATH)
}
