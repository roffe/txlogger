package datalogger

import (
	"bufio"
	"encoding/binary"
	"math"
	"os"
	"time"
)

// BPL (Binary Packed Logfile) on-disk layout. All multi-byte integers are
// little-endian.
//
//	Header:
//	  Magic    [3]byte = "BPL"
//	  Version  uint8   = bplVersion
//	  ColCount uint16            number of value columns
//	  Columns  []Column          repeated ColCount times
//	    NameLen uint16
//	    Name    [NameLen]byte    UTF-8 column name
//	Records (repeated until EOF):
//	  TimeUnixNano int64
//	  Values       [ColCount]float32
//
// The column set is captured from the first Write (same as the CSV header) and
// is assumed stable for the lifetime of the file. Values are stored as float32
// to keep the file compact; this is plenty of precision for the logged signals
// (the textual formats only keep 2-3 decimals anyway).
const (
	bplMagic   = "BPL"
	bplVersion = 1
)

func NewBPLWriter(f *os.File) *BPLWriter {
	return &BPLWriter{
		file: f,
		bw:   bufio.NewWriter(f),
	}
}

type BPLWriter struct {
	file          *os.File
	bw            *bufio.Writer
	headerWritten bool
	colCount      int
	buf           []byte // reusable per-record encode buffer
}

func (b *BPLWriter) Write(ts time.Time, channels []Channel) error {
	if !b.headerWritten {
		if err := b.writeHeader(channels); err != nil {
			return err
		}
	}

	off := 0
	binary.LittleEndian.PutUint64(b.buf[off:], uint64(ts.UnixNano()))
	off += 8

	for i := range channels {
		binary.LittleEndian.PutUint32(b.buf[off:], math.Float32bits(float32(channels[i].Value())))
		off += 4
	}

	_, err := b.bw.Write(b.buf[:off])
	return err
}

func (b *BPLWriter) writeHeader(channels []Channel) error {
	cols := make([]string, 0, len(channels))
	for i := range channels {
		cols = append(cols, channels[i].Name)
	}

	if _, err := b.bw.WriteString(bplMagic); err != nil {
		return err
	}
	if err := b.bw.WriteByte(bplVersion); err != nil {
		return err
	}

	var u16 [2]byte
	binary.LittleEndian.PutUint16(u16[:], uint16(len(cols)))
	if _, err := b.bw.Write(u16[:]); err != nil {
		return err
	}
	for _, name := range cols {
		binary.LittleEndian.PutUint16(u16[:], uint16(len(name)))
		if _, err := b.bw.Write(u16[:]); err != nil {
			return err
		}
		if _, err := b.bw.WriteString(name); err != nil {
			return err
		}
	}

	b.colCount = len(cols)
	b.buf = make([]byte, 8+b.colCount*4)
	b.headerWritten = true
	return nil
}

func (b *BPLWriter) Close() error {
	if err := b.bw.Flush(); err != nil {
		return err
	}
	if err := b.file.Sync(); err != nil {
		return err
	}
	return b.file.Close()
}
