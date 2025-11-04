package ipc

import (
	"context"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/roffe/txlogger/pkg/debug"
)

const pipe = `\\.\pipe\txlogger`

func IsRunning() bool {
	if !ping() {
		debug.Log("txlogger is not running, starting new instance")
	} else {
		debug.Log("txlogger is running, sending show request over socket")
		sendShow()
		return true
	}
	return false
}

func dial() (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return winio.DialPipeContext(ctx, pipe)
}

func listen() (net.Listener, error) {
	return winio.ListenPipe(pipe, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;WD)", // world-read/write; tighten for prod
		InputBufferSize:    1 << 20,           // 1 MiB
		OutputBufferSize:   1 << 20,
	})
}
