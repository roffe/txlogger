package debug

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

var initOnce sync.Once
var fh *os.File

func start() {
	dir, err := os.UserHomeDir()
	if err != nil {
		log.Println("error getting user home dir: %w", err)
	} else {
		dir = filepath.Join(dir, "txlogger")
	}
	fh, err = os.OpenFile(filepath.Join(dir, "txlogger.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("error opening file: %v", err)
	}
}

func Log(msg string) {
	initOnce.Do(start)
	timeStr := time.Now().Format("2006-01-02 15:04:05.000")
	_, fullPath, line, ok := runtime.Caller(1)
	filename := filepath.Base(fullPath)
	if ok {
		LogRaw(fmt.Sprintf("%s %s:%d %s", timeStr, filename, line, msg))
	} else {
		LogRaw(timeStr + " " + msg)
	}
}

func LogDepth(depth int, msg string) {
	initOnce.Do(start)
	timeStr := time.Now().Format("2006-01-02 15:04:05.000")
	_, fullPath, line, ok := runtime.Caller(depth)
	filename := filepath.Base(fullPath)
	if ok {
		LogRaw(fmt.Sprintf("%s %s:%d %s", timeStr, filename, line, msg))
	} else {
		LogRaw(timeStr + " " + msg)
	}
}

func LogRaw(msg string) {
	if fh == nil {
		log.Println("debug file not open")
		return
	}
	fh.WriteString(msg + "\n")
	fmt.Println(msg)
}
func Close() {
	fh.Sync()
	fh.Close()
}
