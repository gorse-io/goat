package add

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestAdd(t *testing.T) {
	a := int64(1)
	b := int64(2)
	c := add(unsafe.Pointer(uintptr(a)), unsafe.Pointer(uintptr(b)))
	assert.Equal(t, a+b, c)
}
