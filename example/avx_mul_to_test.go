package example

import (
	"reflect"
	"testing"
	"unsafe"
)

func AVXMulTo(a, b, c []float32) {
	if len(a) != len(b) || len(a) != len(c) {
		panic("floats: slice lengths do not match")
	}
	avx_mul_to(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0]), unsafe.Pointer(&c[0]), unsafe.Pointer(uintptr(len(a))))
}

func TestAVXMulTo(t *testing.T) {
	a := []float32{1, 2, 3, 4}
	b := []float32{5, 6, 7, 8}
	c := make([]float32, len(a))
	AVXMulTo(a, b, c)
	if !reflect.DeepEqual(c, []float32{5, 12, 21, 32}) {
		t.Errorf("AVXMulTo(%v, %v, %v) = %v, want %v", a, b, c, c, []float32{5, 12, 21, 32})
	}
}
