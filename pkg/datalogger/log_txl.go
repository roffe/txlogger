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
	buf  []byte // reusable per-line buffer
}

func (t *TXWriter) Write(ts time.Time, channels []Channel) error {
	t.buf = ts.AppendFormat(t.buf[:0], "02-01-2006 15:04:05.999")
	t.buf = append(t.buf, '|')
	for i := range channels {
		t.buf = append(t.buf, channels[i].Name...)
		t.buf = append(t.buf, '=')
		// Format the value straight into buf, then swap the first decimal '.'
		// for ',' in place (the TXL/European separator). No per-sample string.
		start := len(t.buf)
		t.buf = channels[i].Append(t.buf)
		for j := start; j < len(t.buf); j++ {
			if t.buf[j] == '.' {
				t.buf[j] = ','
				break
			}
		}
		t.buf = append(t.buf, '|')
	}
	t.buf = append(t.buf, "IMPORTANTLINE=0|\n"...)
	_, err := t.file.Write(t.buf)
	return err
}

func (t *TXWriter) Close() error {
	if err := t.file.Sync(); err != nil {
		return err
	}
	return t.file.Close()
}
