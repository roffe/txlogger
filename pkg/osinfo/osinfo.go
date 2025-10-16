package osinfo

import (
	"log"
	"syscall"
	"unsafe"
)

type RTL_OSVERSIONINFOEXW struct {
	OSVersionInfoSize uint32
	MajorVersion      uint32
	MinorVersion      uint32
	BuildNumber       uint32
	PlatformId        uint32
	CSDVersion        [128]uint16
	ServicePackMajor  uint16
	ServicePackMinor  uint16
	SuiteMask         uint16
	ProductType       byte
	Reserved          byte
}

func RtlGetVersion() RTL_OSVERSIONINFOEXW {
	ntdll := syscall.NewLazyDLL("ntdll.dll")
	rtlGetVersion := ntdll.NewProc("RtlGetVersion")
	var info RTL_OSVERSIONINFOEXW
	info.OSVersionInfoSize = 5*4 + 128*2 + 3*2 + 2*1
	r0, _, err := rtlGetVersion.Call(uintptr(unsafe.Pointer(&info)))
	if r0 != 0 {
		log.Println(err)
	}
	return info
}

//ver := RtlGetVersion()
//if ver.MajorVersion < 10 {
//	sdialog.Message("txlogger requires Windows 10 or later").Title("Unsupported Windows version").Error()
//	return
//}
