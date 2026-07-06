//go:build windows

package client

import (
	"net"
	"os/exec"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio"
)

// hideConsole stops the proxy from flashing a console window.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// dialProxy connects to the proxy's named pipe.
func dialProxy(pipeName string) (net.Conn, error) {
	timeout := 5 * time.Second
	return winio.DialPipe(pipeName, &timeout)
}
