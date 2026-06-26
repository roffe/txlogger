//go:build !windows

package cangw

import "os/exec"

func hideWindow(*exec.Cmd) {}
