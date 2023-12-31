package datalogger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/roffe/txlogger/pkg/symbol"
)

func createLog(path, extension string) (*os.File, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.Mkdir(path, 0755); err != nil {
			if err != os.ErrExist {
				return nil, "", fmt.Errorf("failed to create logs dir: %w", err)
			}
		}
	}
	filename := fmt.Sprintf("log-%s.%s", time.Now().Format("2006-01-02_1504"), extension)
	file, err := os.OpenFile(fullPath(path, filename), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open file: %w", err)
	}
	return file, filename, nil
}

func produceLogLine(file io.Writer, sysvars *ThreadSafeMap, vars []*symbol.Symbol, ts time.Time, sysvarOrder []string) {
	file.Write([]byte(ts.Format("02-01-2006 15:04:05.999") + "|"))
	sysvars.Lock()
	for _, k := range sysvarOrder {
		file.Write([]byte(k + "=" + replaceDot(sysvars.values[k]) + "|"))
	}
	sysvars.Unlock()
	for _, va := range vars {
		file.Write([]byte(va.Name + "=" + replaceDot(va.StringValue()) + "|"))
	}
	file.Write([]byte("IMPORTANTLINE=0|\n"))
}

func replaceDot(s string) string {
	return strings.Replace(s, ".", ",", 1)
}
