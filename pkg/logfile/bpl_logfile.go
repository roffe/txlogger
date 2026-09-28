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

func NewFromBPLLogfile(reader io.Reader) (Logfile, error) {
	r := bufio.NewReader(reader)

	magic := make([]byte, len(bplMagic))
	if _, err := io.ReadFull(r, magic); err != nil {
		return nil, fmt.Errorf("failed to read BPL magic: %w", err)
	}
	if string(magic) != bplMagic {
		return nil, fmt.Errorf("not a BPL logfile (bad magic %q)", magic)
	}

	version, err := r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("failed to read BPL version: %w", err)
	}
	if version != bplVersion {
		return nil, fmt.Errorf("unsupported BPL version %d", version)
	}

	var colCount uint16
	if err := binary.Read(r, binary.LittleEndian, &colCount); err != nil {
		return nil, fmt.Errorf("failed to read column count: %w", err)
	}

	t := newTable()
	cols := make([]int, colCount)
	for i := range cols {
		var nameLen uint16
		if err := binary.Read(r, binary.LittleEndian, &nameLen); err != nil {
			return nil, fmt.Errorf("failed to read column name length: %w", err)
		}
		name := make([]byte, nameLen)
		if _, err := io.ReadFull(r, name); err != nil {
			return nil, fmt.Errorf("failed to read column name: %w", err)
		}
		cols[i] = t.column(string(name))
	}

	recSize := 8 + int(colCount)*4
	recBuf := make([]byte, recSize)
	for {
		if _, err := io.ReadFull(r, recBuf); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				// Clean end of file, or a partially written trailing record.
				break
			}
			return nil, fmt.Errorf("failed to read record: %w", err)
		}

		row := t.addRow(time.Unix(0, int64(binary.LittleEndian.Uint64(recBuf[0:8]))))
		off := 8
		for _, c := range cols {
			t.cols[c][row] = float64(math.Float32frombits(binary.LittleEndian.Uint32(recBuf[off:])))
			off += 4
		}
	}
	return newBaseLogfile(t), nil
}
