#include <arm_bf16.h>

void convert_float32_to_bf16(float *a, void *b, long n) {
	bfloat16_t *bf16_b = (bfloat16_t *)b;
	for (long i = 0; i < n; i++) {
		bf16_b[i] = (bfloat16_t)a[i];
	}
}

void convert_bf16_to_float32(void *a, float *b, long n) {
	bfloat16_t *bf16_a = (bfloat16_t *)a;
	for (long i = 0; i < n; i++) {
		b[i] = (float)bf16_a[i];
	}
}

void add_bf16(void *a, void *b, void *result, long n) {
    bfloat16_t *bf16_a = (bfloat16_t *)a;
	bfloat16_t *bf16_b = (bfloat16_t *)b;
	bfloat16_t *bf16_c = (bfloat16_t *)result;
	for (int i = 0; i < n; i++) {
		bf16_c[i] = bf16_a[i] + bf16_b[i];
	}
}
