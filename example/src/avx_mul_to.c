#include <immintrin.h>
#include <stdint.h>

void avx_mul_to(float *a, float *b, float *c, int64_t n)
{
    int epoch = n / 8;
    int remain = n % 8;
    for (int i = 0; i < epoch; i++)
    {
        __m256 v1 = _mm256_loadu_ps(a);
        __m256 v2 = _mm256_loadu_ps(b);
        __m256 v = _mm256_mul_ps(v1, v2);
        _mm256_storeu_ps(c, v);
        a += 8;
        b += 8;
        c += 8;
    }
    for (int i = 0; i < remain; i++)
    {
        c[i] = a[i] * b[i];
    }
}
