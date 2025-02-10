//go:build windows

package datalogger

import (
	"log"
	"os"
	"path/filepath"
)

var LOGPATH = "logs\\"

func init() {

	if dir, err := os.UserHomeDir(); err == nil {
		LOGPATH = filepath.Join(dir, "txlogger", "logs")
		log.Println("LOGPATH: ", LOGPATH)
	}
	if err := os.MkdirAll(LOGPATH, os.ModePerm); err != nil {
		log.Println("Error creating log directory: ", err)
	}
}
