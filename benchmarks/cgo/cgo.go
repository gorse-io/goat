package cgo

/*
#cgo CFLAGS: -march=armv8.2-a+bf16 -O3
#include <arm_bf16.h>
#include <stdint.h>

void convert_float32_to_bf16(float *a, uint16_t *b, int n) {
	bfloat16_t *bf16_b = (bfloat16_t *)b;
	for (int i = 0; i < n; i++) {
		bf16_b[i] = (bfloat16_t)a[i];
	}
}

void convert_bf16_to_float32(uint16_t *a, float *b, int n) {
	bfloat16_t *bf16_a = (bfloat16_t *)a;
	for (int i = 0; i < n; i++) {
		b[i] = (float)bf16_a[i];
	}
}

void add_bf16(uint16_t *a, uint16_t *b, uint16_t *c, int n) {
	bfloat16_t *bf16_a = (bfloat16_t *)a;
	bfloat16_t *bf16_b = (bfloat16_t *)b;
	bfloat16_t *bf16_c = (bfloat16_t *)c;
	for (int i = 0; i < n; i++) {
		bf16_c[i] = bf16_a[i] + bf16_b[i];
	}
}
*/
import "C"

func ConvertFloat32ToBF16(a []float32) []uint16 {
	b := make([]uint16, len(a))
	n := C.int(len(a))
	C.convert_float32_to_bf16((*C.float)(&a[0]),
		(*C.uint16_t)(&b[0]),
		n)
	return b
}

func ConvertBF16ToFloat32(a []uint16) []float32 {
	b := make([]float32, len(a))
	n := C.int(len(a))
	C.convert_bf16_to_float32((*C.uint16_t)(&a[0]),
		(*C.float)(&b[0]),
		n)
	return b
}

func AddBF16(a, b, c []uint16) {
	if len(a) != len(b) || len(a) != len(c) {
		panic("slices must have the same length")
	}
	n := C.int(len(a))
	C.add_bf16((*C.uint16_t)(&a[0]),
		(*C.uint16_t)(&b[0]),
		(*C.uint16_t)(&c[0]),
		n)
}
