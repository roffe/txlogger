package debug

import (
	"log"
	"os"
	"time"
)

var f *os.File

func init() {
	var err error
	f, err = os.OpenFile("debug.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("error opening file: %v", err)
	}
}

func Log(msg string) {
	LogRaw(time.Now().Format("2006-01-02 15:04:05.000") + " " + msg)
}

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
