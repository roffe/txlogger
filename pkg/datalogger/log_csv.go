package datalogger

import (
	"encoding/csv"
	"math"
	"os"
	"strconv"
	"time"

	symbol "github.com/roffe/ecusymbol"
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
	precission    int
}

func (c *CSVWriter) Write(sysvars *ThreadSafeMap, sysvarOrder []string, vars []*symbol.Symbol, ts time.Time) error {
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
		if va.Number < 0 {
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
		if va.Number < 0 {
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
