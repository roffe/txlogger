//go:build windows

package client

import (
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// dialProxy connects to the proxy's named pipe.
func dialProxy(pipeName string) (net.Conn, error) {
	timeout := 5 * time.Second
	return winio.DialPipe(pipeName, &timeout)
}
