package datalogger

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	symbol "github.com/roffe/ecusymbol"
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
		tx := &TXWriter{
			file: file,
		}
		return filename, tx, nil
	}
	return "unknown", nil, fmt.Errorf("unknown format: %s", cfg.LogFormat)
}

func createLog(path, prefix, extension string) (*os.File, string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.Mkdir(path, 0755); err != nil {
			if err != os.ErrExist {
				return nil, "", fmt.Errorf("failed to create logs dir: %w", err)
			}
		}
	}

	filename := fmt.Sprintf("%s-%s.%s", strings.Replace(prefix, ".", "_", -1), time.Now().Format("2006-01-02_150405"), extension)

	file, err := os.OpenFile(filepath.Join(path, filename), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open file: %w", err)
	}
	return file, filename, nil
}

func NewCSVWriter(f *os.File) *CSVWriter {
	return &CSVWriter{
		file: f,
		cw:   csv.NewWriter(f),
	}
}

type CSVWriter struct {
	file          *os.File
	headerWritten bool
	cw            *csv.Writer
	precission    int
}

func (c *CSVWriter) Write(sysvars *ThreadSafeMap, vars []*symbol.Symbol, ts time.Time, sysvarOrder []string) error {
	if !c.headerWritten {
		if err := c.writeHeader(vars, sysvarOrder); err != nil {
			return err
		}
	}
	var record []string
	record = append(record, ts.Format(ISONICO))
	for _, k := range sysvarOrder {
		val := sysvars.Get(k)
		if val == math.Trunc(val) {
			c.precission = 0
		} else if k == "Lambda.External" {
			c.precission = 3
		} else {
			c.precission = 2
		}
		record = append(record, strconv.FormatFloat(val, 'f', c.precission, 64))
	}
	for _, va := range vars {
		if va.Skip {
			continue
		}
		record = append(record, va.StringValue())
	}
	return c.cw.Write(record)
}

func (c *CSVWriter) writeHeader(vars []*symbol.Symbol, sysvarOrder []string) error {
	var header []string
	header = append(header, "Time")
	header = append(header, sysvarOrder...)
	for _, va := range vars {
		if va.Skip {
			continue
		}
		header = append(header, va.Name)
	}
	c.headerWritten = true
	return c.cw.Write(header)
}

func (c *CSVWriter) Close() error {
	c.cw.Flush()
	if err := c.file.Sync(); err != nil {
		return err
	}
	return c.file.Close()
}

type TXWriter struct {
	file       *os.File
	precission int
}

func (t *TXWriter) Write(sysvars *ThreadSafeMap, vars []*symbol.Symbol, ts time.Time, sysvarOrder []string) error {
	_, err := t.file.Write([]byte(ts.Format("02-01-2006 15:04:05.999") + "|"))
	if err != nil {
		return err
	}
	for _, k := range sysvarOrder {
		val := sysvars.Get(k)
		if val == math.Trunc(val) {
			t.precission = 0
		} else if k == "Lambda.External" {
			t.precission = 3
		} else {
			t.precission = 2
		}
		t.file.Write([]byte(k + "=" + replaceDot(strconv.FormatFloat(val, 'f', t.precission, 64)) + "|"))
	}
	for _, va := range vars {
		if va.Skip {
			continue
		}
		t.file.Write([]byte(va.Name + "=" + replaceDot(va.StringValue()) + "|"))
	}
	_, err = t.file.Write([]byte("IMPORTANTLINE=0|\n"))
	return err
}

func (t *TXWriter) Close() error {
	if err := t.file.Sync(); err != nil {
		return err
	}
	return t.file.Close()
}

func replaceDot(s string) string {
	return strings.Replace(s, ".", ",", 1)
}
