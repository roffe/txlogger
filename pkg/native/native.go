package native

import (
	"errors"
	"reflect"
	"unsafe"
)

var (
	ErrCancelled = errors.New("cancelled")
)

func utf16ptr(utf16 []uint16) *uint16 {
	if utf16[len(utf16)-1] != 0 {
		panic("refusing to make ptr to non-NUL terminated utf16 slice")
	}
	h := (*reflect.SliceHeader)(unsafe.Pointer(&utf16))
	return (*uint16)(unsafe.Pointer(h.Data))
}
