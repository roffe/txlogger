//go:build !linux

package widgets

import "github.com/roffe/browse"

func dialog(op string, o browse.Options) ([]string, error) {
	return showDialog(op, o)
}

// RunFileChild is only used on Linux, where file dialogs are shown by a child
// process.
func RunFileChild() bool {
	return false
}
