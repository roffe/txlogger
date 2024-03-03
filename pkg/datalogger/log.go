package datalogger

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"time"

	symbol "github.com/roffe/ecusymbol"
)

func NewWriter(cfg Config) (string, LogWriter, error) {
	switch cfg.LogFormat {
	case "CSV":
		file, filename, err := createLog(cfg.LogPath, "csv")
		if err != nil {
			return "", nil, err
		}
		return filename, NewCSVWriter(file), nil
	case "TXL":
		file, filename, err := createLog(cfg.LogPath, strings.ToLower(cfg.ECU)+"l")
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
		record = append(record, sysvars.values[k])
	}
	for _, va := range vars {
		record = append(record, va.StringValue())
	}

	return c.cw.Write(record)
}

func (c *CSVWriter) writeHeader(vars []*symbol.Symbol, sysvarOrder []string) error {
	var header []string
	header = append(header, "Time")
	header = append(header, sysvarOrder...)
	for _, va := range vars {
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
	file *os.File
}

func (t *TXWriter) Write(sysvars *ThreadSafeMap, vars []*symbol.Symbol, ts time.Time, sysvarOrder []string) error {
	_, err := t.file.Write([]byte(ts.Format("02-01-2006 15:04:05.999") + "|"))
	if err != nil {
		return err
	}
	sysvars.Lock()
	for _, k := range sysvarOrder {
		t.file.Write([]byte(k + "=" + replaceDot(sysvars.values[k]) + "|"))
	}
	sysvars.Unlock()
	for _, va := range vars {
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
