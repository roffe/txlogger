//go:build windows

package cangw

import (
	"os/exec"
	"syscall"
)

// hideWindow runs the console child without spawning a console window, so
// cangateway can be a console-subsystem exe (no -H=windowsgui, which trips AV
// heuristics) while staying invisible to the user.
func hideWindow(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000 // CREATE_NO_WINDOW
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
}
