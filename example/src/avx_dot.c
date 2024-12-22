#include <immintrin.h>
#include <stdint.h>

void avx_dot(float *a, float *b, int64_t n, float *ret)
{
    int epoch = n / 8;
    int remain = n % 8;
    __m256 s;
    if (epoch > 0)
    {
        __m256 v1 = _mm256_loadu_ps(a);
        __m256 v2 = _mm256_loadu_ps(b);
        s = _mm256_mul_ps(v1, v2);
        a += 8;
        b += 8;
    }
    for (int i = 1; i < epoch; i++)
    {
        __m256 v1 = _mm256_loadu_ps(a);
        __m256 v2 = _mm256_loadu_ps(b);
        s = _mm256_add_ps(_mm256_mul_ps(v1, v2), s);
        a += 8;
        b += 8;
    }
    __m128 s7_6_5_4 = _mm256_extractf128_ps(s, 1);
    __m128 s3_2_1_0 = _mm256_castps256_ps128(s);
    __m128 s37_26_15_04 = _mm_add_ps(s7_6_5_4, s3_2_1_0);
    __m128 sxx_15_04 = s37_26_15_04;
    __m128 sxx_37_26 = _mm_movehl_ps(s37_26_15_04, s37_26_15_04);
    const __m128 sxx_1357_0246 = _mm_add_ps(sxx_15_04, sxx_37_26);
    const __m128 sxxx_0246 = sxx_1357_0246;
    const __m128 sxxx_1357 = _mm_shuffle_ps(sxx_1357_0246, sxx_1357_0246, 0x1);
    __m128 sxxx_01234567 = _mm_add_ss(sxxx_0246, sxxx_1357);
    *ret = _mm_cvtss_f32(sxxx_01234567);
    for (int i = 0; i < remain; i++)
    {
        *ret += a[i] * b[i];
    }
}
