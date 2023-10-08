package logfile

import (
	"bufio"
	"errors"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type TxLogfile struct {
	records []*Record
	length  int
	pos     int
	// mu      sync.Mutex
	// timeFormat string
}

func NewFromTxLogfile(filename string) (Logfile, error) {
	//	start := time.Now()
	rec, err := parseTxLogfile(filename)
	if err != nil {
		return nil, err
	}
	//	log.Printf("Parsed %d records in %s", len(rec), time.Since(start))
	txlog := &TxLogfile{
		records: rec,
		length:  len(rec),
		pos:     -1,
	}

	return txlog, nil
}

func (l *TxLogfile) Next() *Record {
	//l.mu.Lock()
	//defer l.mu.Unlock()
	if l.pos+1 > l.length-1 || l.pos+1 < 0 {
		return nil
	}
	l.pos++
	return l.records[l.pos]
}

func (l *TxLogfile) Prev() *Record {
	//l.mu.Lock()
	//defer l.mu.Unlock()
	if l.pos-1 < 0 {
		return nil
	}
	l.pos--
	return l.records[l.pos]
}

func (l *TxLogfile) Seek(pos int) *Record {
	//l.mu.Lock()
	//defer l.mu.Unlock()
	if pos < 0 || pos >= l.length {
		return nil
	}
	l.pos = pos
	return l.records[pos]
}

func (l *TxLogfile) Pos() int {
	//l.mu.Lock()
	//defer l.mu.Unlock()
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

var timeFormats = []string{
	`02/01/2006 15:04:05.999`,
	`2006/01/02 15:04:05.999`,
	`02-01-2006 15:04:05.999`,
	`2006-01-02 15:04:05.999`,
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

func parseTxLogfile(filename string) ([]*Record, error) {
	lines, err := readTxLogfile(filename)
	if err != nil {
		return nil, err
	}
	noLines := len(lines)

	if noLines <= 0 {
		return nil, errors.New("no lines in file")
	}

	timeFormat, err := detectTimeFormat(lines[0])
	if err != nil {
		return nil, err
	}

	records := make([]*Record, noLines)
	semChan := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup

	for pos := 0; pos < noLines; pos++ {
		semChan <- struct{}{}
		wg.Add(1)
		go func(position int) {
			defer wg.Done()
			if record, err := parseLine(lines[position], timeFormat); err == nil {
				if position+1 < noLines {
					record.DelayTillNext = getDelayTillNext(lines[position+1], timeFormat, record.Time)
				}
				records[position] = record
			} else {
				log.Println(err)
			}
			<-semChan
		}(pos)
	}
	wg.Wait()
	return records, nil
}

func parseLine(line, timeFormat string) (*Record, error) {
	parsedTime, rawValues, err := splitTxLogLine(line, timeFormat)
	if err != nil {
		return nil, err
	}
	record := NewRecord(parsedTime)
	for _, kv := range rawValues {
		if strings.HasPrefix(kv, "IMPORTANTLINE") {
			continue
		}
		key, value, err := parseValue(kv)
		if err != nil {
			return nil, err
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

func parseValue(valueString string) (string, float64, error) {
	parts := strings.Split(valueString, "=")
	val, err := strconv.ParseFloat(strings.Replace(parts[1], ",", ".", 1), 64)
	if err != nil {
		return "", 0, err
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

func readTxLogfile(filename string) ([]string, error) {
	readFile, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer readFile.Close()
	fileScanner := bufio.NewScanner(readFile)
	var output []string
	for fileScanner.Scan() {
		output = append(output, fileScanner.Text())
	}
	return output, nil
}
