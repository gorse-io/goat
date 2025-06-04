package asm

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestAdd(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{4.0, 5.0, 6.0}
	a1 := make([]uint16, len(a))
	b1 := make([]uint16, len(b))
	c1 := make([]uint16, len(a))
	convert_float32_to_bf16(unsafe.Pointer(&a[0]), unsafe.Pointer(&a1[0]), int64(len(a)))
	convert_float32_to_bf16(unsafe.Pointer(&b[0]), unsafe.Pointer(&b1[0]), int64(len(b)))
	add_bf16(unsafe.Pointer(&a1[0]), unsafe.Pointer(&b1[0]), unsafe.Pointer(&c1[0]), int64(len(a)))
	c := make([]float32, len(a))
	convert_bf16_to_float32(unsafe.Pointer(&c1[0]), unsafe.Pointer(&c[0]), int64(len(c)))
	assert.Equal(t, []float32{5.0, 7.0, 9.0}, c)
}
