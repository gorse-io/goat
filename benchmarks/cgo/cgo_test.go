package cgo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdd(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0}
	b := []float32{4.0, 5.0, 6.0}
	a1 := ConvertFloat32ToBF16(a)
	b1 := ConvertFloat32ToBF16(b)
	c1 := make([]uint16, len(a1))
	AddBF16(a1, b1, c1)
	c := ConvertBF16ToFloat32(c1)
	assert.Equal(t, []float32{5.0, 7.0, 9.0}, c)
}
