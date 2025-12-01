package datalogger

import (
	"math"
	"os"
	"strconv"
	"time"

	symbol "github.com/roffe/ecusymbol"
)

func NewTXLWriter(f *os.File) *TXWriter {
	return &TXWriter{
		file: f,
	}
}

type TXWriter struct {
	file       *os.File
	precission int
}

func (t *TXWriter) Write(sysvars *ThreadSafeMap, sysvarOrder []string, vars []*symbol.Symbol, ts time.Time) error {
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
		if _, err := t.file.Write([]byte(k + "=" + replaceDot(strconv.FormatFloat(val, 'f', t.precission, 64)) + "|")); err != nil {
			return err
		}
	}
	for _, va := range vars {
		if va.Number < 0 {
			continue
		}
		if _, err := t.file.Write([]byte(va.Name + "=" + replaceDot(va.StringValue()) + "|")); err != nil {
			return err
		}
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
