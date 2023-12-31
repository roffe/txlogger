//go:build linux
// +build linux

package datalogger

import (
	"fmt"
	"strings"
)

const (
	LOGPATH = "logs/"
)

func fullPath(path, filename string) string {
	return fmt.Sprintf("%s/%s", strings.TrimSuffix(path, "/"), filename)
}
