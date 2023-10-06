package datalogger

import (
	"fmt"
	"os"
	"time"
)

func createLog(extension string) (*os.File, string, error) {
	/*
		dir, err := getLogDir()
		if err != nil {
			return nil, "", err
		}
	*/
	if _, err := os.Stat("logs"); os.IsNotExist(err) {
		if err := os.Mkdir("logs", 0755); err != nil {
			if err != os.ErrExist {
				return nil, "", fmt.Errorf("failed to create logs dir: %w", err)
			}
		}
	}
	filename := fmt.Sprintf("log-%s.%s", time.Now().Format("2006-01-02_1504"), extension)
	file, err := os.OpenFile(LOGPATH+filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open file: %w", err)
	}
	return file, filename, nil
}
