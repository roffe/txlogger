package settings

import (
	"runtime"
	"slices"
	"testing"

	"go.bug.st/serial/enumerator"
)

func TestPortInfo(t *testing.T) {
	names := []string{"ttyUSB10", "COM3", "ttyUSB2", "ttyACM0", "COM12"}
	slices.SortFunc(names, comparePortNames)
	if want := []string{"COM3", "COM12", "ttyACM0", "ttyUSB2", "ttyUSB10"}; !slices.Equal(names, want) {
		t.Errorf("order %v, want %v", names, want)
	}

	canusb := newPortInfo("/dev/serial/by-id/usb-LAWICEL_CANUSB_LWSXKNIO-if00-port0", "ttyUSB0", &enumerator.PortDetails{
		Product: "CANUSB", Manufacturer: "LAWICEL", SerialNumber: "LWSXKNIO", VID: "0403", PID: "6001",
	})
	if canusb.title != "CANUSB" || canusb.detail != "ttyUSB0 · LAWICEL · S/N LWSXKNIO · 0403:6001" {
		t.Errorf("got %q / %q", canusb.title, canusb.detail)
	}

	win := newPortInfo("COM5", "COM5", &enumerator.PortDetails{Product: "USB Serial Port (COM5)", VID: "1A86", PID: "7523"})
	if win.title != "USB Serial Port" || win.detail != "COM5 · 1a86:7523" {
		t.Errorf("got %q / %q", win.title, win.detail)
	}

	if bare := newPortInfo("/dev/ttyS4", "ttyS4", &enumerator.PortDetails{}); bare.title != "Serial port" || bare.detail != "ttyS4" {
		t.Errorf("got %q / %q", bare.title, bare.detail)
	}

	ports := []portInfo{win, canusb}
	if runtime.GOOS == "windows" {
		if findPort(ports, " com5 ") != 0 {
			t.Error("COM5 not found case-insensitively")
		}
		return
	}
	if findPort(ports, canusb.path) != 1 || findPort(ports, "/dev/ttyUSB9") != -1 {
		t.Error("findPort by path")
	}
}
