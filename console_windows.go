package main

import (
	"errors"
	"log"
	"os"
	"syscall"

	"github.com/roffe/gocan/pkg/w32"
)

const (
	ATTACH_PARENT_PROCESS = ^uint32(0) // (DWORD)-1
)

var (
	procAttachConsole              = w32.Modkernel32.MustFindProc("AttachConsole")
	oldStdin, oldStdout, oldStderr = os.Stdin, os.Stdout, os.Stderr //lint:ignore U1000 Prevent GC of the original std handles
)

func InitConsole() {
	ok, err := attachConsole(ATTACH_PARENT_PROCESS)
	if ok {
		log.Println("attaching console")
		hout, err1 := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
		if err1 != nil {
			log.Printf("stdout connection error : %v", err1)
		}
		herr, err2 := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE)
		if err2 != nil {
			log.Printf("stderr connection error : %v", err2)
		}
		os.Stdout = os.NewFile(uintptr(hout), "/dev/stdout")
		os.Stderr = os.NewFile(uintptr(herr), "/dev/stderr")
		log.SetOutput(os.Stderr)
		log.Println("attached console")
		return
	}
	if err != nil {
		if errors.Is(err, syscall.Errno(5)) {
			// Access denied means already attached to a console
			return
		}
		log.Printf("attachConsole failed: %v", err)
	}

}

func attachConsole(dwParentProcess uint32) (ok bool, lasterr error) {
	r1, _, lasterr := syscall.SyscallN(procAttachConsole.Addr(), uintptr(dwParentProcess), 0, 0)
	ok = bool(r1 != 0)
	return
}
