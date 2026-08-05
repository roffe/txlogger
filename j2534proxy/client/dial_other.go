//go:build !windows

package client

import (
	"errors"
	"net"
	"os/exec"
)

func hideConsole(*exec.Cmd) {}

func dialProxy(string) (net.Conn, error) {
	return nil, errors.New("j2534proxy is windows only")
}
