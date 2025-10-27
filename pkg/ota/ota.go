package ota

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"github.com/roffe/gocan/pkg/serialcommand"
	"go.bug.st/serial"
	"golang.org/x/mod/semver"
)

//go:embed firmware.bin
var firmware []byte

const (
	COM_SPEED              = 1000000
	MinimumtxbridgeVersion = "1.0.9"
)

type Config struct {
	Port         string
	Logfunc      func(...any)
	ProgressFunc func(float64)
}

func UpdateOTA(cfg Config) error {
	if cfg.Logfunc == nil {
		cfg.Logfunc = log.Println
	}
	if cfg.ProgressFunc == nil {
		cfg.ProgressFunc = func(progress float64) {
			cfg.Logfunc(fmt.Sprintf("Progress: %.1f%%", progress))
		}
	}
	if cfg.Port == "" {
		return fmt.Errorf("port is empty")
	}

	start := time.Now()
	cfg.Logfunc("Opening " + cfg.Port + " port")

	var err error
	var sp io.ReadWriteCloser

	if strings.HasPrefix(cfg.Port, "tcp://") {
		sp, err = openTcpPort(cfg.Port[6:])
	} else {
		sp, err = openSerialPort(cfg.Port)
	}

	if err != nil {
		if sp != nil {
			sp.Close()
		}
		return err
	}
	defer sp.Close()

	//firmware, err := os.ReadFile(cfg.Filename)
	//if err != nil {
	//	return err
	//}

	//cfg.Logfunc("Firmware size: ", len(firmware))

	cmd := serialcommand.NewSerialCommand('v', []byte{0x10})
	buf, err := cmd.MarshalBinary()
	if err != nil {
		return err
	}

	if _, err := sp.Write(buf); err != nil {
		return err
	}

	cmd, err = readSerialCommand(sp, 5*time.Second)
	if err != nil {
		return err
	}

	if err := checkErr(cmd); err != nil {
		return fmt.Errorf("OTA update failed: %v", err)
	}

	if cmd.Command != 'v' {
		return fmt.Errorf("unexpected response: %X %X", cmd.Command, cmd.Data)
	}

	cfg.Logfunc("Device firmware: ", string(cmd.Data))

	ver := semver.Compare("v"+string(cmd.Data), "v"+MinimumtxbridgeVersion)
	if ver == 0 {
		cc := make(chan bool)
		fyne.Do(func() {
			dialog.ShowConfirm("Firmware up to date", "Firmware is already up to date, install anyway?", func(b bool) {
				cc <- b
			}, fyne.CurrentApp().Driver().AllWindows()[0])
		})
		t := <-cc
		if !t {
			return errors.New("OTA update aborted")
		}

	}

	if _, err := sp.Write([]byte{'u'}); err != nil {
		return err
	}

	cmd, err = readSerialCommand(sp, 4*time.Second)
	if err != nil {
		return err
	}
	if cmd.Command == 'e' {
		return fmt.Errorf("device is not ready for OTA: %X", cmd.Data)
	}

	var firmwarePtr int
	if cmd.Command == 'u' {
		totalSize := len(firmware)
		var progress float64
		cfg.Logfunc("Uploading firmware..")
		for firmwarePtr < totalSize {
			// print progress as % of firmware sent
			progress = float64(firmwarePtr) / float64(totalSize) * 100
			cfg.ProgressFunc(progress)
			end := firmwarePtr + 250
			if end > len(firmware) {
				end = len(firmware)
			}
			data := firmware[firmwarePtr:end]
			err := writeSerialCommand(sp, 'U', data)
			if err != nil {
				return err
			}
			firmwarePtr += len(data)

			cmd, err := readSerialCommand(sp, 2*time.Second)
			if err != nil {
				return fmt.Errorf("finish firmware upload failed: %v", err)
			}
			if err := checkErr(cmd); err != nil {
				return err
			}
			if cmd.Command != 'U' {
				return fmt.Errorf("unexpected response OTA update: %X %X", cmd.Command, cmd.Data)
			}

		}
		cfg.ProgressFunc(100.0)
	} else {
		return fmt.Errorf("unexpected response: %X %X", cmd.Command, cmd.Data)
	}

	if _, err := sp.Write([]byte{'F'}); err != nil {
		return err
	}

	cmd, err = readSerialCommand(sp, 1*time.Second)
	if err != nil {
		return err
	}

	if err := checkErr(cmd); err != nil {
		return fmt.Errorf("OTA update failed: %v", err)
	}

	if cmd.Command == 'F' && len(cmd.Data) == 2 && bytes.Equal(cmd.Data, []byte{0x6F, 0x6B}) {
		cfg.Logfunc("OTA finished in ", time.Since(start).Truncate(100*time.Millisecond))
		return nil
	}

	return fmt.Errorf("unexpected response end OTA: %X %X", cmd.Command, cmd.Data)
}

func checkErr(cmd *serialcommand.SerialCommand) error {
	if cmd.Command == 'e' {
		return fmt.Errorf("error: %X %X", cmd.Command, cmd.Data)
	}
	return nil
}

func openSerialPort(port string) (io.ReadWriteCloser, error) {
	mode := &serial.Mode{
		BaudRate: COM_SPEED,
	}
	p, err := serial.Open(port, mode)
	if err != nil {
		return nil, err
	}
	p.SetReadTimeout(20 * time.Millisecond)
	return p, nil
}

func openTcpPort(address string) (io.ReadWriteCloser, error) {
	d := net.Dialer{Timeout: 2 * time.Second}
	if address == "" {
		address = "192.168.4.1:1337"
	}
	if value := os.Getenv("TXBRIDGE_ADDRESS"); value != "" {
		address = value
	}
	if !strings.HasSuffix(address, ":1337") {
		address += ":1337" // Ensure the port is always set
	}
	p, err := d.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	// if t, ok := p.(*net.TCPConn); ok {
	// 	t.SetNoDelay(true)
	// }

	return p, nil
}

// readSerialCommand reads a single command from the serial port with timeout
func readSerialCommand(port io.ReadWriteCloser, timeout time.Duration) (*serialcommand.SerialCommand, error) {

	var (
		parsingCommand  bool
		command         byte
		commandSize     byte
		commandChecksum byte
		cmdbuff         = make([]byte, 256)
		cmdbuffPtr      byte
	)

	readbuf := make([]byte, 16)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		n, err := port.Read(readbuf)
		if err != nil {
			return nil, fmt.Errorf("read error: %w", err)
		}
		if n == 0 {
			continue
		}

		for _, b := range readbuf[:n] {
			if !parsingCommand {
				parsingCommand = true
				command = b
				continue
			}

			if commandSize == 0 {
				commandSize = b
				continue
			}

			if cmdbuffPtr == commandSize {
				if commandChecksum != b {
					log.Printf("data: %X", cmdbuff[:cmdbuffPtr])
					return nil, fmt.Errorf("checksum error: expected %02X, got %02X", b, commandChecksum)
				}

				data := make([]byte, cmdbuffPtr)
				copy(data, cmdbuff[:cmdbuffPtr])

				return &serialcommand.SerialCommand{
					Command: command,
					Data:    data,
				}, nil
			}

			if cmdbuffPtr < commandSize {
				cmdbuff[cmdbuffPtr] = b
				cmdbuffPtr++
				commandChecksum += b
			}
		}
	}
	return nil, fmt.Errorf("timeout after %v", timeout)
}

// writeSerialCommand writes a single command to the serial port
func writeSerialCommand(port io.ReadWriteCloser, command byte, data []byte) error {
	cmd := &serialcommand.SerialCommand{
		Command: command,
		Data:    data,
	}

	buf, err := cmd.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	_, err = port.Write(buf)
	if err != nil {
		return fmt.Errorf("write error: %w", err)
	}

	return nil
}
