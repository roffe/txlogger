//go:build windows
// +build windows

package datalogger

import (
	"fmt"
	"strings"
)

const (
	LOGPATH = "logs\\"
)

func fullPath(path, filename string) string {
	return fmt.Sprintf("%s\\%s", strings.TrimSuffix(path, "\\"), filename)
}
