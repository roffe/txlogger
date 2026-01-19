package stag

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.bug.st/serial"
)

const ProductString = "Stag AFR"

type STAG struct {
	port string
	sp   serial.Port

	lambda float64
	oxygen float64

	log func(string)

	closeOnce sync.Once
	mu        sync.Mutex
	worker    *workerInfo
}

func NewSTAGClient(port string, logFunc func(string)) (*STAG, error) {
	return &STAG{
		port: port,
		log:  logFunc,
	}, nil
}

func (a *STAG) Start(ctx context.Context) error {

	mode := &serial.Mode{
		BaudRate: 57600,
	}
	sp, err := serial.Open(a.port, mode)
	if err != nil {
		return err
	}
	a.sp = sp

	a.sp.SetReadTimeout(5 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	a.worker = &workerInfo{
		cancel: cancel,
		done:   make(chan struct{}),
	}
	go func() {
		a.run(ctx)
		close(a.worker.done)
	}()

	return nil
}

func (a *STAG) GetLambda() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lambda
}

func (a *STAG) run(ctx context.Context) {
	packetContentBuffer := make([]byte, 0, 64)
	buf := make([]byte, 8)
	packetStarted := false
	byteCounter := 0
	packetSize := 0

	// Create a channel to receive bytes
	byteChan := make(chan byte, 100)
	errChan := make(chan error, 1)

	a.sendRequest([]byte{0xAC, 0x00, 0x00, 0x04, 0x00, 0x00, 0x32, 0xE2})
	// Start a goroutine to read bytes
	go func() {
		for {
			// read from serial
			n, err := a.sp.Read(buf)
			if ctx.Err() != nil {
				return
			}
			if n == 0 {
				continue
			}

			if err != nil {
				errChan <- err
			}
			for _, b := range buf[:n] {
				byteChan <- b
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errChan:
			a.log(err.Error())
			// Handle errorfunc
			return
		case aByte := <-byteChan:
			if !packetStarted && aByte == 0x32 {
				packetContentBuffer = packetContentBuffer[:0] // Clear buffer
				packetContentBuffer = append(packetContentBuffer, aByte)
				packetStarted = true
				byteCounter = 1
			} else {
				packetContentBuffer = append(packetContentBuffer, aByte)
				byteCounter++
				if byteCounter == 4 {
					packetSize = int(aByte) + 4
				}
				if packetSize == byteCounter {
					packetStarted = false
					a.processPacket(packetContentBuffer)
				}
			}
		}
	}
}

func (a *STAG) Stop() {
	a.closeOnce.Do(func() {
		if a.sp != nil {
			a.log("Stopping Stag serial client")
			if err := a.sp.Close(); err != nil {
				a.log(err.Error())
			}
		}
	})
}
func (a *STAG) processPacket(packetContentBuffer []byte) {
	if len(packetContentBuffer) < 5 {
		return
	}

	switch packetContentBuffer[4] {
	case 0x80:
		a.sendRequest([]byte{0x32, 0x00, 0x00, 0x03, 0x03, 0x00, 0x38})
	case 0x83:
		a.sendRequest([]byte{0x32, 0x00, 0x00, 0x03, 0x6D, 0x00, 0xA2})
	case 0xF0:
		a.sendRequest([]byte{0x32, 0x00, 0x00, 0x03, 0x64, 0x00, 0x99})
	case 0xE4:
		a.SetData(packetContentBuffer)
		a.sendRequest([]byte{0x32, 0x00, 0x00, 0x03, 0x64, 0x00, 0x99})
	default:
		// Not handled
	}
}

func (a *STAG) sendRequest(data []byte) {
	time.Sleep(100 * time.Millisecond)
	a.sp.Write(data)
}

func (a *STAG) SetData(data []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch data[6] {
	case 0x00:
		a.log("status_sleep")
	case 0x01:
		a.log("status_warming")
	case 0x02:
		// status_work
		a.lambda = float64(uint32(data[12])<<24|uint32(data[13])<<16|uint32(data[14])<<8|uint32(data[15])) * 0.001
		a.oxygen = float64((uint16(data[16])<<8)|uint16(data[17])) * 0.1
	case 0x03:
		a.log("status_breakdown")
	default:
	}
	return nil
}

func (a *STAG) String() string {
	return fmt.Sprintf("Lambda: %.4f, Oxygen: %.3f", a.lambda, a.oxygen)
}

type workerInfo struct {
	cancel context.CancelFunc
	done   chan struct{}
}
