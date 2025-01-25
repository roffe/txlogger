package logfile

import (
	"bufio"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

var _ Logfile = (*TxLogfile)(nil)

type TxLogfile struct {
	BaseLogfile
}

func NewFromTxLogfile(filename string) (Logfile, error) {
	txlog := &TxLogfile{}
	txlog.pos = -1
	// start := time.Now()
	if err := txlog.parseTxLogfile(filename); err != nil {
		return nil, err
	}
	// log.Printf("Parsed %d records in %s", len(rec), time.Since(start))
	return txlog, nil
}

var timeFormats = []string{
	`02/01/2006 15:04:05.999`,
	`2006/01/02 15:04:05.999`,
	`02-01-2006 15:04:05.999`,
	`2006-01-02 15:04:05.999`,
	`02.01.2006 15:04:05.999`,
}

func detectTimeFormat(text string) (string, error) {
	text = strings.Split(strings.TrimSuffix(text, "|"), "|")[0]
	for _, format := range timeFormats {
		if _, err := time.Parse(format, text); err == nil {
			return format, nil
		}
	}
	return "", errors.New("could not detect time format")
}

func (l *TxLogfile) parseTxLogfile(filename string) error {
	lines := make([]string, 0)
	readFile, err := os.Open(filename)
	if readFile != nil {
		defer readFile.Close()
	}
	if err != nil {
		return err
	}
	buffer := make([]byte, 4*1024)
	fileScanner := bufio.NewScanner(readFile)
	fileScanner.Buffer(buffer, bufio.MaxScanTokenSize)
	for fileScanner.Scan() {
		lines = append(lines, string(fileScanner.Bytes()))
	}

	noLines := len(lines)

	if noLines <= 0 {
		return errors.New("no lines in file")
	}

	timeFormat, err := detectTimeFormat(lines[0])
	if err != nil {
		return err
	}

	l.records = make([]Record, noLines)
	for pos := 0; pos < noLines; pos++ {
		if record, err := parseLine(lines[pos], timeFormat); err == nil {
			if pos+1 < noLines {
				record.DelayTillNext = getDelayTillNext(lines[pos+1], timeFormat, record.Time)
			}
			l.records[pos] = record
		} else {
			log.Println(err)
		}
	}

	l.length = len(l.records)
	l.end = l.length - 1
	return nil
}

func parseLine(line, timeFormat string) (Record, error) {
	parsedTime, rawValues, err := splitTxLogLine(line, timeFormat)
	if err != nil {
		return Record{}, err
	}
	record := NewRecord(parsedTime)
	for _, kv := range rawValues {
		if strings.HasPrefix(kv, "IMPORTANTLINE") {
			continue
		}
		key, value, err := parseCommaValue(kv)
		if err != nil {
			return Record{}, err
		}
		record.SetValue(key, value)
	}
	return record, nil
}

func getDelayTillNext(line, timeFormat string, currentDelay time.Time) int64 {
	pipeIndex := strings.Index(line, "|")
	if pipeIndex != -1 {
		textBeforePipe := line[:pipeIndex]
		parsedTime, err := time.Parse(timeFormat, textBeforePipe)
		if err != nil {
			log.Println(err)
			return 0
		}
		return parsedTime.Sub(currentDelay).Milliseconds()
	}
	return 0
}

func parseCommaValue(valueString string) (string, float64, error) {
	parts := strings.Split(valueString, "=")
	val, err := strconv.ParseFloat(strings.Replace(parts[1], ",", ".", 1), 64)
	if err != nil {
		return "", -1, err
	}
	return parts[0], val, nil
}

func splitTxLogLine(line, timeFormat string) (time.Time, []string, error) {
	touples := strings.Split(strings.TrimSuffix(line, "|"), "|")
	parsedTime, err := time.Parse(timeFormat, touples[0])
	if err != nil {
		return time.Time{}, nil, err
	}
	return parsedTime, touples[1:], nil
}
