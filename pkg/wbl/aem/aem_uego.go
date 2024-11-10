package aem

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"sync"
	"time"

	"go.bug.st/serial"
)

const ProductString = "AEM Uego"

type AEMuego struct {
	port string
	sp   serial.Port

	lamba   float64
	oxygen  float64
	voltage float64

	log func(string)

	closeOnce sync.Once
	mu        sync.Mutex

	dataBuff []byte
	dataPos  int
}

func NewAEMuegoClient(port string, logFunc func(string)) (*AEMuego, error) {
	return &AEMuego{
		port:     port,
		log:      logFunc,
		dataBuff: make([]byte, 8),
	}, nil
}

func (a *AEMuego) Start(ctx context.Context) error {

	mode := &serial.Mode{
		BaudRate: 9600,
	}
	sp, err := serial.Open(a.port, mode)
	if err != nil {
		return err
	}
	a.sp = sp

	a.sp.SetReadTimeout(5 * time.Millisecond)

	go a.run(ctx)

	return nil
}

func (a *AEMuego) GetLambda() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lamba
}

func (a *AEMuego) SetLambda(value float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lamba = value
}

func (a *AEMuego) run(ctx context.Context) {
	buf := make([]byte, 8)
	for {
		// read from serial
		n, err := a.sp.Read(buf)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			a.log("AEM: " + err.Error())
			return
		}
		if n == 0 {
			continue
		}

		//log.Printf("AEM: %s", buf[:n])

		for _, b := range buf[:n] {
			switch b {
			case '\r':
				continue
			case '\n':
				value, err := strconv.ParseFloat(string(a.dataBuff[:a.dataPos]), 64)
				if err != nil {
					a.log("AEM: " + err.Error())
					a.dataPos = 0
					continue
				}

				// log.Printf("AEM: %0.3f", value/10)
				a.mu.Lock()
				a.lamba = value / 10
				a.mu.Unlock()

				a.dataPos = 0
				continue
			}
			a.dataBuff[a.dataPos] = b
			a.dataPos++
			if a.dataPos == 8 {
				a.dataPos = 0
			}
		}

	}
}

func (a *AEMuego) Stop() {
	a.closeOnce.Do(func() {
		if a.sp != nil {
			a.log("Stopping AEM serial client")
			if err := a.sp.Close(); err != nil {
				a.log(err.Error())
			}
		}
	})
}

/*
| Byte | Bit    | Bitmask | Label               | Data Type       | Scaling             | Offset | Range
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
|  0-1 |        |         | Lambda              | 16 bit unsigned | .0001 Lambda/bit    | 0      | 0 to 6.5535 Lambda
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
|  2-3 |        |         | Oxygen              | 16 bit signed   | 0.001%/bit          | 0      | -32.768% to 32.767%
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
|   4  |        |         | System Volts        | 8 bit unsigned  | 0.1 V/bit           | 0      | 0 to 25.5 Volts
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
|   5  |        |         | Reserved            | ---             | ---                 | ---    | ---
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
|   6  |0 (lsb) |    0    | Reserved            | ---             | ---                 | ---    | ---
|      |   1    |    2    | AEM/FAE Detected    | Boolean         | 0 = false, 1 = true | 0      | 0/1
|      |  2-4   |    4    | Reserved            | ---             | ---                 | ---    | ---
|      |   5    |   32    | Free-Air cal in use | Boolean         | 0 = false, 1 = true | 0      | 0/1
|      |   6    |   64    | Reserved            | ---             | ---                 | ---    | ---
|      |7 (msb) |  128    | Lambda Data Valid   | Boolean         | 0 = false, 1 = true | 0      | 0/1
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
|   7  |  0-5   |    0    | Reserved            | ---             | ---                 | ---    | ---
|      |   6    |   64    | Sensor Fault        | Boolean         | 0 = false, 1 = true | 0      |0/1
|      | 7(msb) |  128    | Reserved            | ---             | ---                 | ---    | ---
|------|--------|---------|---------------------|-----------------|---------------------|--------|---------------------
*/

// Set CAN data
func (a *AEMuego) SetData(data []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	r := bytes.NewReader(data)
	var wbl uint16
	binary.Read(r, binary.BigEndian, &wbl)
	a.lamba = float64(wbl) * 0.0001

	var oxygen uint16
	binary.Read(r, binary.BigEndian, &oxygen)
	a.oxygen = float64(oxygen) * 0.001

	var systemVolt uint8
	binary.Read(r, binary.BigEndian, &systemVolt)
	a.voltage = float64(systemVolt) * 0.1

	return nil
}

func (a *AEMuego) String() string {
	return fmt.Sprintf("Lambda: %.4f, Oxygen: %.3f, Voltage: %.1f", a.lamba, a.oxygen, a.voltage)
}
