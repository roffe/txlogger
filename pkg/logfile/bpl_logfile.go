package logfile

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"time"
)

// BPL is Binary Packed Logfile, a custom binary format for storing log data efficiently.
// It is designed to be compact and fast to read/write, making it suitable for large log files.
//
// See pkg/datalogger/log_bpl.go (BPLWriter) for the authoritative description of
// the on-disk layout. In short: a header with a magic, version and the list of
// value column names, followed by fixed-width records of an int64 unix-nano
// timestamp and one float32 per column.
const (
	bplMagic   = "BPL"
	bplVersion = 1
)

var _ Logfile = (*BPLLogfile)(nil)

type BPLLogfile struct {
	BaseLogfile
}

func NewFromBPLLogfile(reader io.Reader) (Logfile, error) {
	b := &BPLLogfile{}
	b.pos = -1
	if err := b.parseBPLLogfile(reader); err != nil {
		return nil, err
	}
	return b, nil
}

func (l *BPLLogfile) parseBPLLogfile(reader io.Reader) error {
	r := bufio.NewReader(reader)

	magic := make([]byte, len(bplMagic))
	if _, err := io.ReadFull(r, magic); err != nil {
		return fmt.Errorf("failed to read BPL magic: %w", err)
	}
	if string(magic) != bplMagic {
		return fmt.Errorf("not a BPL logfile (bad magic %q)", magic)
	}

	version, err := r.ReadByte()
	if err != nil {
		return fmt.Errorf("failed to read BPL version: %w", err)
	}
	if version != bplVersion {
		return fmt.Errorf("unsupported BPL version %d", version)
	}

	var colCount uint16
	if err := binary.Read(r, binary.LittleEndian, &colCount); err != nil {
		return fmt.Errorf("failed to read column count: %w", err)
	}

	cols := make([]string, colCount)
	for i := range cols {
		var nameLen uint16
		if err := binary.Read(r, binary.LittleEndian, &nameLen); err != nil {
			return fmt.Errorf("failed to read column name length: %w", err)
		}
		name := make([]byte, nameLen)
		if _, err := io.ReadFull(r, name); err != nil {
			return fmt.Errorf("failed to read column name: %w", err)
		}
		cols[i] = string(name)
	}

	recSize := 8 + int(colCount)*4
	recBuf := make([]byte, recSize)
	for {
		if _, err := io.ReadFull(r, recBuf); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				// Clean end of file, or a partially written trailing record.
				break
			}
			return fmt.Errorf("failed to read record: %w", err)
		}

		ts := time.Unix(0, int64(binary.LittleEndian.Uint64(recBuf[0:8])))
		rec := NewRecord(ts)
		off := 8
		for _, c := range cols {
			bits := binary.LittleEndian.Uint32(recBuf[off:])
			rec.SetValue(c, float64(math.Float32frombits(bits)))
			off += 4
		}
		l.records = append(l.records, rec)
	}

	for i := 0; i < len(l.records)-1; i++ {
		l.records[i].DelayTillNext = l.records[i+1].Time.Sub(l.records[i].Time).Milliseconds()
	}

	l.length = len(l.records)
	l.end = l.length - 1
	return nil
}
