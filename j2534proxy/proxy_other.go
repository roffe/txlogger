//go:build !windows

package main

import "errors"

func run() error {
	return errors.New("j2534proxy only runs on windows")
}
