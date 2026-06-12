package datalogger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/roffe/txlogger/pkg/common"
)

func NewWriter(cfg Config) (string, LogWriter, error) {
	switch cfg.LogFormat {
	case "CSV":
		file, filename, err := createLog(cfg.LogPath, cfg.FilenamePrefix, "csv")
		if err != nil {
			return "", nil, err
		}
		return filename, NewCSVWriter(file), nil
	case "TXL":
		file, filename, err := createLog(cfg.LogPath, cfg.FilenamePrefix, strings.ToLower(cfg.ECU)+"l")
		if err != nil {
			return "", nil, err
		}
		return filename, NewTXLWriter(file), nil
	case "BPL":
		file, filename, err := createLog(cfg.LogPath, cfg.FilenamePrefix, "bpl")
		if err != nil {
			return "", nil, err
		}
		return filename, NewBPLWriter(file), nil
	}
	return "unknown", nil, fmt.Errorf("unknown format: %s", cfg.LogFormat)
}

func createLog(path, prefix, extension string) (*os.File, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.Mkdir(path, 0o755); err != nil {
			if err != os.ErrExist {
				return nil, "", fmt.Errorf("failed to create logs dir: %w", err)
			}
		}
	}

	filename := fmt.Sprintf("%s-%s.%s", strings.ReplaceAll(prefix, ".", "_"), time.Now().Format("2006-01-02_150405"), extension)
	filename = common.SanitizeFilename(filename)

	fullFilename := filepath.Join(path, filename)

	file, err := os.OpenFile(fullFilename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open file: %w", err)
	}
	return file, fullFilename, nil
}

func replaceDot(s string) string {
	return strings.Replace(s, ".", ",", 1)
}
