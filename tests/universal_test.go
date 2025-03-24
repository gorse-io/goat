package tests

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestAdd(t *testing.T) {
	a := int64(1)
	b := int64(2)
	c := add(a, b)
	assert.Equal(t, a+b, c)
}

func TestL2(t *testing.T) {
	a := []float32{1, 2, 3, 4}
	b := []float32{5, 6, 7, 8}
	c := l2(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0]), int64(len(a)))
	assert.Equal(t, float32(64), c)
}

func TestMatMul(t *testing.T) {
	a := []float32{1, 2, 3, 4}
	b := []float32{5, 6, 7, 8}
	res := make([]float32, 4)
	mat_mul(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0]), unsafe.Pointer(&res[0]), 2, 2, 2)
	assert.Equal(t, []float32{19, 22, 43, 50}, res)
}
