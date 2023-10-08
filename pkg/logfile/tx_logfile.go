package logfile

import (
	"bufio"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type TxLogfile struct {
	records []*Record
	length  int
	pos     int
	mu      sync.Mutex
	// timeFormat string
}

func NewFromTxLogfile(filename string) (Logfile, error) {
	start := time.Now()
	rec, err := parseTxLogfile(filename)
	if err != nil {
		return nil, err
	}
	log.Printf("Parsed %d records in %s", len(rec), time.Since(start))
	txlog := &TxLogfile{
		records: rec,
		length:  len(rec),
		pos:     0,
	}

	return txlog, nil
}

func (l *TxLogfile) Next() *Record {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pos+1 > l.length-1 {
		return nil
	}
	l.pos++
	return l.records[l.pos]
}

func (l *TxLogfile) Prev() *Record {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pos-1 < 0 {
		return nil
	}
	l.pos--
	return l.records[l.pos]
}

func (l *TxLogfile) Seek(pos int) *Record {
	l.mu.Lock()
	defer l.mu.Unlock()
	if pos < 0 || pos >= l.length {
		return nil
	}
	l.pos = pos
	return l.records[pos]
}

func (l *TxLogfile) Pos() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.pos
}

func (l *TxLogfile) SeekTime(time.Time) *Record {
	return nil
}

func (l *TxLogfile) Len() int {
	return l.length
}

func (l *TxLogfile) Start() time.Time {
	if l.length > 0 {
		return l.records[0].Time
	}
	return time.Time{}
}

func (l *TxLogfile) End() time.Time {
	if l.length > 0 {
		return l.records[l.length-1].Time
	}
	return time.Time{}
}

func detectTimeFormat(text string) (string, error) {
	var formats = []string{
		"02/01/2006 15:04:05.999",
		"02-01-2006 15:04:05.999",
	}
	text = strings.Split(strings.TrimSuffix(text, "|"), "|")[0]
	for _, format := range formats {
		if _, err := time.Parse(format, text); err == nil {
			log.Printf("Detected time format: %s", format)
			return format, nil
		}
	}
	return "", errors.New("could not detect time format")
}

func parseTxLogfile(filename string) ([]*Record, error) {
	lines, err := readTxLogfile(filename)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, errors.New("no lines in file")
	}

	timeFormat, err := detectTimeFormat(lines[0])
	if err != nil {
		return nil, err
	}

	noLines := len(lines)

	var records []*Record
	for pos, line := range lines {
		parsedTime, rawValues, err := splitTxLogLine(line, timeFormat)
		if err != nil {
			log.Println(err)
			continue
		}

		record := &Record{
			Time:   parsedTime,
			Values: make([]*RecordValue, 0),
		}

		if pos+1 < noLines {

			pipeIndex := strings.Index(lines[pos+1], "|")

			// Check if "|" character exists in the string
			if pipeIndex != -1 {
				// Extract the text before the first "|"
				textBeforePipe := lines[pos+1][:pipeIndex]
				parsedTime2, err := time.Parse(timeFormat, textBeforePipe)
				if err != nil {
					log.Println(err)
					continue
				}
				record.DelayTillNext = parsedTime2.Sub(parsedTime).Milliseconds()
			}
		}

		for _, kv := range rawValues {
			parts := strings.Split(kv, "=")
			if parts[0] == "IMPORTANTLINE" {
				continue
			}
			val, err := strconv.ParseFloat(strings.Replace(parts[1], ",", ".", 1), 64)
			if err != nil {
				log.Println(err)
				continue
			}
			record.Values = append(record.Values, &RecordValue{
				Key:   parts[0],
				Value: val,
			})
		}
		records = append(records, record)
	}

	return records, nil
}

func splitTxLogLine(line, timeFormat string) (time.Time, []string, error) {
	touples := strings.Split(strings.TrimSuffix(line, "|"), "|")
	parsedTime, err := time.Parse(timeFormat, touples[0])
	if err != nil {
		return time.Time{}, nil, err
	}
	return parsedTime, touples[1:], nil
}

func readTxLogfile(filename string) ([]string, error) {
	start := time.Now()
	readFile, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer readFile.Close()

	fileScanner := bufio.NewScanner(readFile)
	fileScanner.Split(bufio.ScanLines)

	var output []string
	for fileScanner.Scan() {
		output = append(output, fileScanner.Text())
	}
	log.Printf("Read %d lines in %s", len(output), time.Since(start))
	return output, nil
}
