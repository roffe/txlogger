package debug

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

func init() {
	var err error
	f, err = os.OpenFile("debug.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("error opening file: %v", err)
	}
}

func Log(msg string) {
	timeStr := time.Now().Format("2006-01-02 15:04:05.000")
	_, fullPath, line, ok := runtime.Caller(2)
	filename := filepath.Base(fullPath)
	if ok {
		LogRaw(fmt.Sprintf("%s %s:%d %s", timeStr, filename, line, msg))
	} else {
		LogRaw(timeStr + " " + msg)
	}
}

var f *os.File

func LogRaw(msg string) {
	if f == nil {
		var err error
		f, err = os.OpenFile("debug.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			log.Println("error opening file: %w", err)
			return
		}
	}
	f.WriteString(msg + "\n")
}
func Close() {
	f.Sync()
	f.Close()
}
