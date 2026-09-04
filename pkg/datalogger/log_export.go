package datalogger

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/roffe/txlogger/pkg/logfile"
)

// ExportRecords writes the given records to a new logfile in dir, picking the
// writer from ext (one of "csv", "bpl", "t5l", "t7l", "t8l") so the exported
// clip keeps the same format as the log it was cut from. The filename is built
// from prefix plus a timestamp by createLog. It returns the full path of the
// created file.
func ExportRecords(dir, prefix, ext string, records []logfile.Record) (string, error) {
	if len(records) == 0 {
		return "", errors.New("no records to export")
	}

	ext = strings.ToLower(strings.TrimPrefix(ext, "."))

	file, filename, err := createLog(dir, prefix, ext)
	if err != nil {
		return "", err
	}

	var w LogWriter
	switch ext {
	case "csv":
		w = NewCSVWriter(file)
	case "bpl":
		w = NewBPLWriter(file)
	case "t5l", "t7l", "t8l":
		w = NewTXLWriter(file)
	default:
		file.Close()
		return "", fmt.Errorf("unsupported export format: %s", ext)
	}

	cols := recordColumns(records[0])

	// One channel set is built up front; the backing values are rewritten for
	// each record so we don't allocate a closure per cell.
	values := make([]float64, len(cols))
	channels := make([]Channel, len(cols))
	for i, name := range cols {
		i := i
		channels[i] = Channel{
			Name:      name,
			read:      func() float64 { return values[i] },
			appendFmt: sysvarFormat(name),
		}
	}

	for i := range records {
		rec := records[i]
		for j, name := range cols {
			values[j] = rec.Values[name]
		}
		if err := w.Write(rec.Time, channels); err != nil {
			_ = w.Close()
			return "", fmt.Errorf("failed to write record: %w", err)
		}
	}

	if err := w.Close(); err != nil {
		return "", fmt.Errorf("failed to finalize log: %w", err)
	}
	return filename, nil
}

// recordColumns returns the value column names of a record in a stable
// (alphabetical) order so the exported log has a deterministic layout.
func recordColumns(rec logfile.Record) []string {
	cols := make([]string, 0, len(rec.Values))
	for k := range rec.Values {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}
