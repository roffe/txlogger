// Command j2534proxy serves 32-bit J2534 PassThru DLLs to the 64-bit
// txlogger over a localhost TCP connection (see the protocol package).
// txlogger launches it at startup on Windows, holds its stdin open as a
// lifeline and pings it; the proxy exits by itself when the pings stop or
// stdin reaches EOF, so it can never outlive the main process.
//
// Build it 32-bit so the DLLs load:
//
//	GOOS=windows GOARCH=386 go build -tags j2534 -o j2534proxy.exe ./j2534proxy
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
