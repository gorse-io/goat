package asm

import "unsafe"

func AddBF16(a, b, c []uint16) {
	if len(a) != len(b) || len(a) != len(c) {
		panic("slices must have the same length")
	}
	add_bf16(unsafe.Pointer(&a[0]),
		unsafe.Pointer(&b[0]),
		unsafe.Pointer(&c[0]),
		int64(len(a)))
}
