//go:build !linux

package main

// This is a no-op outside Linux and will never be called because the FP environment variable is only set in the Linux build, but we need it to exist to satisfy the linker.
func runFileChild() {}
