package tests

import (
	"encoding/base64"
	"runtime"
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

func TestLoadConstantPool(t *testing.T) {
	output := make([]int64, 4)
	load_constant_pool(unsafe.Pointer(&output[0]))
	assert.Equal(t, []int64{3, 4, 5, 6}, output)
}

func TestLoadZeroConstantPool(t *testing.T) {
	output := make([]int64, 3)
	load_zero_constant_pool(unsafe.Pointer(&output[0]))
	assert.Equal(t, []int64{0, 15, 16}, output)
}

func TestL2(t *testing.T) {
	a := []float32{1, 2, 3, 4}
	b := []float32{5, 6, 7, 8}
	c := l2(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0]), int64(len(a)))
	assert.Equal(t, float32(64), c)
}

func TestSquaredEuclideanDistanceFP32(t *testing.T) {
	a := []float32{1, 2, 3, 4}
	b := []float32{5, 6, 7, 8}
	result := squared_euclidean_distance_fp32(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0]), int64(len(a)))
	assert.Equal(t, float32(64), result)
}

func TestIndirectJump(t *testing.T) {
	if runtime.GOARCH == "s390x" {
		t.Skip("GoAT's s390x backend does not support Clang conditional returns")
	}
	lhs := []float32{1, 2, 3, 4, 5, 6, 7}
	rhs := []float32{2, 3, 4, 5, 6, 7, 8}
	for _, test := range []struct {
		size     int64
		expected float32
	}{
		{size: 0, expected: 0},
		{size: 1, expected: 2},
		{size: 2, expected: 8},
		{size: 3, expected: 20},
		{size: 4, expected: 40},
		{size: 5, expected: 70},
		{size: 6, expected: 112},
		{size: 7, expected: 168},
		{size: 8, expected: 0},
	} {
		result := indirect_jump(unsafe.Pointer(&lhs[0]), unsafe.Pointer(&rhs[0]), test.size)
		assert.Equal(t, test.expected, result, "size %d", test.size)
	}
}

func TestMatMul(t *testing.T) {
	a := []float32{1, 2, 3, 4}
	b := []float32{5, 6, 7, 8}
	res := make([]float32, 4)
	mat_mul(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0]), unsafe.Pointer(&res[0]), 2, 2, 2)
	assert.Equal(t, []float32{19, 22, 43, 50}, res)
}

func TestMul2(t *testing.T) {
	assert.Equal(t, float32(4), mul2(2))
}

func TestNot(t *testing.T) {
	assert.False(t, _not(true))
}

func TestLoadBool(t *testing.T) {
	value := true
	assert.True(t, load_bool(unsafe.Pointer(&value)))
}

func TestSum(t *testing.T) {
	assert.Equal(t, int64(55), sum(1, 2, 3, 4, 5, 6, 7, 8, 9, 10))
}

func TestMul(t *testing.T) {
	assert.Equal(t, float64(40320), mul(1, 2, 3, 4, 5, 6, 7, 8))
}

func TestReverse(t *testing.T) {
	a := []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	reverse(unsafe.Pointer(&a[0]), unsafe.Pointer(&a[1]), unsafe.Pointer(&a[2]), unsafe.Pointer(&a[3]), unsafe.Pointer(&a[4]),
		unsafe.Pointer(&a[5]), unsafe.Pointer(&a[6]), unsafe.Pointer(&a[7]), unsafe.Pointer(&a[8]), unsafe.Pointer(&a[9]))
	assert.Equal(t, []float32{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}, a)
}

func TestBase64Encode(t *testing.T) {
	ptr := func(b []byte) unsafe.Pointer {
		if len(b) == 0 {
			return nil
		}
		return unsafe.Pointer(&b[0])
	}

	for _, input := range [][]byte{
		{},
		[]byte("f"),
		[]byte("fo"),
		[]byte("foo"),
		[]byte("hello, goat"),
	} {
		dst := make([]byte, base64.StdEncoding.EncodedLen(len(input)))
		n := base64_encode(ptr(input), int64(len(input)), ptr(dst))
		assert.Equal(t, int64(len(dst)), n)
		assert.Equal(t, base64.StdEncoding.EncodeToString(input), string(dst))
	}
}
