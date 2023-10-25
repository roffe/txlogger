package lzhuf

// #cgo CFLAGS: -DLZHUF=1
// #include "lzhuf.h"
import "C"

import (
	"unsafe"
)

func Decode(in []byte, out []byte) int {
	cin := (*C.uchar)(unsafe.Pointer(&in[0]))
	cout := (*C.uchar)(unsafe.Pointer(&out[0]))
	return int(C.Decode(cin, cout))
}
