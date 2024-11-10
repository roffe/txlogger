package main

import (
	"log"
	"os"
	"syscall"
)

const ATTACH_PARENT_PROCESS = ^uint32(0) // (DWORD)-1

var (
	modkernel32       = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole = modkernel32.NewProc("AttachConsole")
)

func attachConsole(dwParentProcess uint32) (ok bool, lasterr error) {
	r1, _, lasterr := syscall.SyscallN(procAttachConsole.Addr(), uintptr(dwParentProcess), 0, 0)
	ok = bool(r1 != 0)
	return
}

/*
func AttachConsole() error {
	const ATTACH_PARENT_PROCESS = ^uintptr(0)
	proc := syscall.MustLoadDLL("kernel32.dll").MustFindProc("AttachConsole")
	r1, _, err := proc.Call(ATTACH_PARENT_PROCESS)
	if r1 == 0 {
		errno, ok := err.(syscall.Errno)
		if ok && errno == w32.ERROR_INVALID_HANDLE {
			// console handle doesn't exist; not a real
			// error, but the console handle will be
			// invalid.
			return nil
		}
		return err
	} else {
		return nil
	}
}
*/

var oldStdin, oldStdout, oldStderr = os.Stdin, os.Stdout, os.Stderr //lint:ignore U1000 Prevent GC of the original std handles

func init() {

	/*
		stdin, _ := syscall.GetStdHandle(syscall.STD_INPUT_HANDLE)
		stdout, _ := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
		stderr, _ := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE)
			var invalid syscall.Handle
			con := invalid

			if stdin == invalid || stdout == invalid || stderr == invalid {
				err := AttachConsole()
				if err != nil {
					log.Printf("attachconsole: %v", err)
					return
				}

				if stdin == invalid {
					stdin, _ = syscall.GetStdHandle(syscall.STD_INPUT_HANDLE)
				}
				if stdout == invalid {
					stdout, _ = syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
					con = stdout
				}
				if stderr == invalid {
					stderr, _ = syscall.GetStdHandle(syscall.STD_ERROR_HANDLE)
					con = stderr
				}
			}

			if con != invalid {
				// Make sure the console is configured to convert
				// \n to \r\n, like Go programs expect.
				h := windows.Handle(con)
				var st uint32
				err := windows.GetConsoleMode(h, &st)
				if err != nil {
					log.Printf("GetConsoleMode: %v", err)
					return
				}
				err = windows.SetConsoleMode(h, st&^windows.DISABLE_NEWLINE_AUTO_RETURN)
				if err != nil {
					log.Printf("SetConsoleMode: %v", err)
					return
				}
			}

			if stdin != invalid {
				os.Stdin = os.NewFile(uintptr(stdin), "stdin")
			}
			if stdout != invalid {
				os.Stdout = os.NewFile(uintptr(stdout), "stdout")
			}
			if stderr != invalid {
				os.Stderr = os.NewFile(uintptr(stderr), "stderr")
			}
	*/

	ok, lasterr := attachConsole(ATTACH_PARENT_PROCESS)
	if ok {
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
		return
	}
	if lasterr != nil {
		log.Printf("attachConsole failed : %v", lasterr)
	}

}
