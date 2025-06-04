package benchmarks

import (
	"strconv"
	"testing"

	"github.com/gorse-io/goat/benchmarks/asm"
	"github.com/gorse-io/goat/benchmarks/cgo"
)

var batchSizes = []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024}

func makeFloat32s(n int) []float32 {
	f := make([]float32, n)
	for i := 0; i < n; i++ {
		f[i] = float32(i)
	}
	return f
}

func BenchmarkFloat32(b *testing.B) {
	for _, n := range batchSizes {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			a := makeFloat32s(n)
			c := makeFloat32s(n)
			d := makeFloat32s(n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j := 0; j < len(a); j++ {
					d[j] = a[j] + c[j]
				}
			}
		})
	}
}

func BenchmarkCGO(b *testing.B) {
	for _, n := range batchSizes {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			a := makeFloat32s(n)
			c := makeFloat32s(n)
			a1 := cgo.ConvertFloat32ToBF16(a)
			c1 := cgo.ConvertFloat32ToBF16(c)
			c2 := make([]uint16, len(a1))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cgo.AddBF16(a1, c1, c2)
			}
		})
	}
}

func BenchmarkGOAT(b *testing.B) {
	for _, n := range batchSizes {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			a := makeFloat32s(n)
			c := makeFloat32s(n)
			a1 := cgo.ConvertFloat32ToBF16(a)
			c1 := cgo.ConvertFloat32ToBF16(c)
			c2 := make([]uint16, len(a1))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				asm.AddBF16(a1, c1, c2)
			}
		})
	}
}
