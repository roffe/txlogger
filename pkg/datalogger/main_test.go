package datalogger

import (
	"fmt"
	"os"
	"testing"
)

// TestMain points the home directory at a throwaway dir for the whole package:
// the loggers look for user Lua scripts under ~/txlogger/scripts (creating the
// directory) and the T5 fast path appends to ~/txlogger/txlogger.log, neither
// of which a test run should touch or depend on.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "datalogger-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
