package datalogger

import (
	"os"
	"time"
)

func NewTXLWriter(f *os.File) *TXWriter {
	return &TXWriter{
		file: f,
	}
}

type TXWriter struct {
	file *os.File
}

func (t *TXWriter) Write(ts time.Time, channels []Channel) error {
	_, err := t.file.Write([]byte(ts.Format("02-01-2006 15:04:05.999") + "|"))
	if err != nil {
		return err
	}
	for i := range channels {
		if _, err := t.file.Write([]byte(channels[i].Name + "=" + replaceDot(channels[i].String()) + "|")); err != nil {
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
