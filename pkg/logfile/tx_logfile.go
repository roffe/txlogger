package logfile

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"log"
	"strconv"
	"time"
)

var timeFormats = []string{
	`02/01/2006 15:04:05.999`,
	`2006/01/02 15:04:05.999`,
	`02-01-2006 15:04:05.999`,
	`2006-01-02 15:04:05.999`,
	`02.01.2006 15:04:05.999`,
}

func detectTimeFormat(text string) (string, error) {
	for _, format := range timeFormats {
		if _, err := time.Parse(format, text); err == nil {
			return format, nil
		}
	}
	return "", errors.New("could not detect time format")
}

// NewFromTxLogfile parses the t5l/t7l/t8l format, one record per line:
//
//	18-07-2025 14:21:20.196|ActualIn.n_Engine=850|Out.fi_Ignition=7,5|IMPORTANTLINE=0|
//
// A line whose time does not parse is skipped, as is a value that does not.
func NewFromTxLogfile(reader io.Reader) (Logfile, error) {
	sc := bufio.NewScanner(reader)
	sc.Buffer(make([]byte, 4*1024), bufio.MaxScanTokenSize)
	t := newTable()
	var timeFormat string
	lines := 0
	for sc.Scan() {
		lines++
		stamp, rest, _ := bytes.Cut(bytes.TrimSuffix(sc.Bytes(), []byte("|")), []byte("|"))
		if timeFormat == "" {
			var err error
			if timeFormat, err = detectTimeFormat(string(stamp)); err != nil {
				return nil, err
			}
		}
		ts, err := time.Parse(timeFormat, string(stamp))
		if err != nil {
			log.Println(err)
			continue
		}
		row := t.addRow(ts)
		for len(rest) > 0 {
			var kv []byte
			kv, rest, _ = bytes.Cut(rest, []byte("|"))
			key, val, ok := bytes.Cut(kv, []byte("="))
			if !ok || bytes.HasPrefix(key, []byte("IMPORTANTLINE")) {
				continue
			}
			// The decimal separator is a comma. The line is ours to change
			// until the next Scan, so swap it in place rather than copy.
			if i := bytes.IndexByte(val, ','); i >= 0 {
				val[i] = '.'
			}
			v, err := strconv.ParseFloat(string(val), 64)
			if err != nil {
				continue
			}
			c, ok := t.index[string(key)]
			if !ok {
				c = t.column(string(key))
			}
			t.cols[c][row] = v
		}
	}
	if lines == 0 {
		return nil, errors.New("no lines in file")
	}
	return newBaseLogfile(t), nil
}
