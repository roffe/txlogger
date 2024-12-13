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
	var err error
	fh, err = os.OpenFile("debug.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("error opening file: %v", err)
	}
}

func Log(msg string) {
	initOnce.Do(start)

	timeStr := time.Now().Format("2006-01-02 15:04:05.000")
	_, fullPath, line, ok := runtime.Caller(2)
	filename := filepath.Base(fullPath)
	if ok {
		LogRaw(fmt.Sprintf("%s %s:%d %s", timeStr, filename, line, msg))
	} else {
		LogRaw(timeStr + " " + msg)
	}
}

func LogRaw(msg string) {
	if fh == nil {
		var err error
		fh, err = os.OpenFile("debug.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			log.Println("error opening file: %w", err)
			return
		}
	}
	fh.WriteString(msg + "\n")
}
func Close() {
	fh.Sync()
	fh.Close()
}
