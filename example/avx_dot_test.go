package example

import (
	"testing"
	"unsafe"
)

func AVXDot(a, b []float32) float32 {
	if len(a) != len(b) {
		panic("floats: slice lengths do not match")
	}
	var c float32
	avx_dot(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0]), unsafe.Pointer(uintptr(len(a))), unsafe.Pointer(&c))
	return c
}

func TestDot(t *testing.T) {
	a := []float32{1, 2, 3, 4}
	b := []float32{5, 6, 7, 8}
	c := AVXDot(a, b)
	if c != 70 {
		t.Errorf("AVXDot(%v, %v) = %v, want %v", a, b, c, 70)
	}
}
