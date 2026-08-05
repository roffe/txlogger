package datalogger

import (
	"encoding/csv"
	"os"
	"time"

	"github.com/roffe/txlogger/pkg/logfile"
)

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

func (c *CSVWriter) Write(ts time.Time, channels []Channel) error {
	if !c.headerWritten {
		if err := c.writeHeader(channels); err != nil {
			return err
		}
	}
	record := make([]string, 0, len(channels)+1)
	record = append(record, ts.Format(logfile.ISONICO))
	for i := range channels {
		record = append(record, channels[i].String())
	}
	return c.cw.Write(record)
}

func (c *CSVWriter) writeHeader(channels []Channel) error {
	header := make([]string, 0, len(channels)+1)
	header = append(header, "Time")
	for i := range channels {
		header = append(header, channels[i].Name)
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
