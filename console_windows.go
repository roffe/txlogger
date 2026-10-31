package main

import (
	"log"
	"os"
	"syscall"
)

const (
	ATTACH_PARENT_PROCESS = ^uint32(0) // (DWORD)-1
)

var (
	modkernel32                    *syscall.DLL
	procAttachConsole              *syscall.Proc
	oldStdin, oldStdout, oldStderr = os.Stdin, os.Stdout, os.Stderr //lint:ignore U1000 Prevent GC of the original std handles
)

func init() {
	var err error
	modkernel32, err = syscall.LoadDLL("kernel32.dll")
	if err != nil {
		panic(err)
	}
	procAttachConsole, err = modkernel32.FindProc("AttachConsole")
	if err != nil {
		panic(err)
	}
}

func InitConsole() {
	r1, _, errno := syscall.SyscallN(procAttachConsole.Addr(), uintptr(ATTACH_PARENT_PROCESS))
	if r1 == 0 {
		// 5 = already have a console, 6 = parent has none (launched from Explorer)
		if errno != 5 && errno != 6 {
			log.Printf("AttachConsole: %v", errno)
		}
		return
	}
	stdout, _ := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	stderr, _ := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE)
	os.Stdout = os.NewFile(uintptr(stdout), "/dev/stdout")
	os.Stderr = os.NewFile(uintptr(stderr), "/dev/stderr")
	log.SetOutput(os.Stderr)
}
