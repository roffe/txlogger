package logfile

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// benchCols/benchRows approximate a real 40 minute T7 session at 20 Hz.
const benchCols, benchRows = 25, 50000

func benchNames() []string {
	names := make([]string, benchCols)
	for i := range names {
		names[i] = fmt.Sprintf("Sym%02d.value_name", i)
	}
	return names
}

func benchValue(r, c int) float64 { return math.Round(1000*math.Sin(float64(r*c+c))) / 100 }

func synthTXL() []byte {
	var b bytes.Buffer
	names := benchNames()
	t0 := time.Date(2026, 9, 27, 12, 0, 0, 0, time.Local)
	for r := range benchRows {
		b.WriteString(t0.Add(time.Duration(r) * 50 * time.Millisecond).Format("02-01-2006 15:04:05.999"))
		b.WriteByte('|')
		for c, n := range names {
			b.WriteString(n)
			b.WriteByte('=')
			b.WriteString(strings.Replace(strconv.FormatFloat(benchValue(r, c), 'f', 2, 64), ".", ",", 1))
			b.WriteByte('|')
		}
		b.WriteString("IMPORTANTLINE=0|\n")
	}
	return b.Bytes()
}

func synthCSV() []byte {
	var b bytes.Buffer
	names := benchNames()
	b.WriteString("Time," + strings.Join(names, ",") + "\n")
	t0 := time.Date(2026, 9, 27, 12, 0, 0, 0, time.Local)
	for r := range benchRows {
		b.WriteString(`"` + t0.Add(time.Duration(r)*50*time.Millisecond).Format(ISONICO) + `"`)
		for c := range names {
			b.WriteByte(',')
			b.WriteString(strconv.FormatFloat(benchValue(r, c), 'f', 2, 64))
		}
		b.WriteByte('\n')
	}
	return b.Bytes()
}

func synthBPL() []byte {
	var b bytes.Buffer
	names := benchNames()
	b.WriteString(bplMagic)
	b.WriteByte(bplVersion)
	binary.Write(&b, binary.LittleEndian, uint16(len(names)))
	for _, n := range names {
		binary.Write(&b, binary.LittleEndian, uint16(len(n)))
		b.WriteString(n)
	}
	t0 := time.Date(2026, 9, 27, 12, 0, 0, 0, time.Local)
	for r := range benchRows {
		binary.Write(&b, binary.LittleEndian, t0.Add(time.Duration(r)*50*time.Millisecond).UnixNano())
		for c := range names {
			binary.Write(&b, binary.LittleEndian, math.Float32bits(float32(benchValue(r, c))))
		}
	}
	return b.Bytes()
}

func heapInUse() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

func benchOpen(b *testing.B, name string, data []byte) {
	// Retained heap of one open log: what the log player keeps for the whole
	// time the log is open. Measured before the loop, since b.Loop keeps the
	// last iteration's result alive.
	before := heapInUse()
	sink, _ = Open(name, bytes.NewReader(data))
	retained := (float64(heapInUse()) - float64(before)) / (1 << 20)
	sink = nil

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		if _, err := Open(name, bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(retained, "MiB-retained")
}

var sink Logfile

func BenchmarkOpenTXL(b *testing.B) { benchOpen(b, "x.t7l", synthTXL()) }
func BenchmarkOpenCSV(b *testing.B) { benchOpen(b, "x.csv", synthCSV()) }
func BenchmarkOpenBPL(b *testing.B) { benchOpen(b, "x.bpl", synthBPL()) }

// BenchmarkOpenFile opens a real log: TXLOG=/path/to/log.t7l go test -bench OpenFile
func BenchmarkOpenFile(b *testing.B) {
	path := os.Getenv("TXLOG")
	if path == "" {
		b.Skip("set TXLOG to a log file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	benchOpen(b, path, data)
}
