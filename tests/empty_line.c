#include <immintrin.h>

#include <immintrin.h>

void l2(float *a, float *b, float *res, long *len)
{
    int n = *len;
    float sum = 0;

    __m256 acc[4];
    acc[0] = _mm256_setzero_ps();
    acc[1] = _mm256_setzero_ps();

    while (n)
    {
        __m256 a_vec0 = _mm256_loadu_ps(a);
        __m256 b_vec0 = _mm256_loadu_ps(b);

        __m256 diff0 = _mm256_sub_ps(a_vec0, b_vec0);

        acc[0] = _mm256_fmadd_ps(diff0, diff0, acc[0]);

        n--;
        a++;
        b++;
    }

    acc[0] = _mm256_add_ps(acc[1], acc[0]);
    if (*len >= 32)
    {
        acc[2] = _mm256_add_ps(acc[3], acc[2]);
        acc[0] = _mm256_add_ps(acc[2], acc[0]);
    }

    __m256 t1 = _mm256_hadd_ps(acc[0], acc[0]);
    __m256 t2 = _mm256_hadd_ps(t1, t1);
    __m128 t3 = _mm256_extractf128_ps(t2, 1);
    __m128 t4 = _mm_add_ps(_mm256_castps256_ps128(t2), t3);
    sum += _mm_cvtss_f32(t4);
    *res = sum;
}
