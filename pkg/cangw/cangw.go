package cangw

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/roffe/txlogger/pkg/debug"
)

const (
	exeName     = "cangateway.exe"
	readyMarker = "server listening"
	readyWait   = 3 * time.Second
)

func Start() (*os.Process, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	cmd := exec.Command(filepath.Join(wd, exeName))

	// Uncomment on Windows if you want to hide the console window:
	// cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	readyCh := make(chan struct{})
	go watchStderr(stderr, readyCh)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", exeName, err)
	}
	log.Printf("started cangateway pid: %d", cmd.Process.Pid)

	ctx, cancel := context.WithTimeout(context.Background(), readyWait)
	defer cancel()

	select {
	case <-readyCh:
		debug.Log("cangateway is ready")
		return cmd.Process, nil
	case <-ctx.Done():
		// Best effort cleanup if it never signaled ready.
		log.Println("context done, kill cangateway.exe")
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("cangateway was not ready after %s", readyWait)
	}
}

func watchStderr(rc io.ReadCloser, readyCh chan<- struct{}) {
	defer rc.Close()

	sc := bufio.NewScanner(rc)
	starting := true
	for sc.Scan() {
		line := sc.Text()
		fmt.Println(line)
		if starting && strings.Contains(line, readyMarker) {
			close(readyCh)
			starting = false
		}
	}

	if err := sc.Err(); err != nil {
		log.Printf("cangateway stderr error: %v", err)
	}
	log.Println("cangateway exited")
}
