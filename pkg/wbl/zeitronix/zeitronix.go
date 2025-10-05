package zeitronix

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"go.bug.st/serial"
)

const ProductString = "Zeitronix ZT-2"

/*
Zeitronix Packet format, []byte
[0] always 0
[1] always 1
[2] always 2
[3] AFR
[4] EGT Low
[5] EGT High
[6] RPM Low
[7] RPM High
[8] MAP Low
[9] MAP High
[10] TPS
[11] USER1
[12] Config Register1
[13] Config Register2
*/

type Zeitronix struct {
	Port string

	lambdaValue float64
	egtValue    uint16
	rpmValue    uint16
	mapValue    uint16

	p         serial.Port
	closeOnce sync.Once
	logFunc   func(string)
}

func NewZeitronixClient(port string, logFunc func(string)) (*Zeitronix, error) {
	z := &Zeitronix{
		Port:    port,
		logFunc: logFunc,
	}
	return z, nil
}

func (z *Zeitronix) Start(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: 9600,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
		DataBits: 8,
	}
	sp, err := serial.Open(z.Port, mode)
	if err != nil {
		return err
	}
	z.p = sp
	z.p.SetReadTimeout(500 * time.Millisecond)
	go z.serialHandler()

	return nil
}

func (z *Zeitronix) serialHandler() {
	buff := make([]byte, 14)
	cmd := make([]byte, 14)
	step := 0
	for {
		n, err := z.p.Read(buff)
		if err != nil {
			log.Println("Zeitronix read error:", err)
			return
		}
		if n == 0 {
			continue
		}
		for _, b := range buff[:n] {
			switch step {
			case 0, 1, 2:
				if b == byte(step) {
					cmd[step] = b
					step++
					continue
				}
				step = 0
				continue
			case 3, 4, 5, 6, 7, 8, 9, 10, 11, 12:
				cmd[step] = b
				step++
				continue
			case 13:
				cmd[13] = b
				// Got full packet parse it
				z.SetData(cmd)
				step = 0
				continue
			default:
				step = 0
			}
		}

	}
}

func (z *Zeitronix) SetData(data []byte) error {
	if len(data) < 14 {
		return errors.New("invalid data length")
	}
	if data[0] != 0 || data[1] != 1 || data[2] != 2 {
		return errors.New("invalid data format")
	}
	z.lambdaValue = float64(data[3]) * 0.01
	z.egtValue = uint16(data[4]) | (uint16(data[5]) << 8)
	z.rpmValue = uint16(data[6]) | (uint16(data[7]) << 8)
	z.mapValue = uint16(data[8]) | (uint16(data[9]) << 8)
	return nil
}

func (z *Zeitronix) Stop() {
	z.closeOnce.Do(func() {
		z.logFunc("Closing Zeitronix client")
		if z.p != nil {
			z.p.Close()
		}
	})
}

func (z *Zeitronix) GetLambda() float64 {
	return z.lambdaValue
}

func (z *Zeitronix) String() string {
	return fmt.Sprintf("Lambda: %.3f, EGT: %d, RPM: %d, MAP: %d", z.lambdaValue, z.egtValue, z.rpmValue, z.mapValue)
}
