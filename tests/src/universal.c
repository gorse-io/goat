long add(long a, long b)
{
    return a + b;
}

float l2(const float *a, const float *b, long n)
{
    float sum = 0;
    for (int i = 0; i < n; i++)
    {
        float diff = a[i] - b[i];
        float sq = diff * diff;
        sum += sq;
    }
    return sum;
}

void mat_mul(float *a, float *b, float *res, long d1, long d2, long d3)
{
    for (int i = 0; i < d1; i++)
    {
        for (int j = 0; j < d3; j++)
        {
            float sum = 0;
            for (int k = 0; k < d2; k++)
            {
                sum += a[i * d2 + k] * b[k * d3 + j];
            }
            res[i * d3 + j] = sum;
        }
    }
}

inline __attribute__((always_inline)) float add_inline(float a, float b)
{
    return a + b;
}

float mul2(float a)
{
    return add_inline(a, a);
}

_Bool _not(_Bool a)
{
    return !a;
}

long sum(long x1, long x2, long x3, long x4, long x5, long x6, long x7, long x8, long x9, long x10)
{
    return x1 + x2 + x3 + x4 + x5 + x6 + x7 + x8 + x9 + x10;
}

double mul(float v1, double v2, float v3, float v4, double v5, double v6, long v7, double v8)
{
    return v1 * v2 * v3 * v4 * v5 * v6 * v7 * v8;
}

void reverse(float *x0, float *x1, float *x2, float *x3, float *x4, float *x5, float *x6, float *x7, float *x8, float *x9, float *x10)
{
    float tmp;
    tmp = *x0; *x0 = *x10; *x10 = tmp;
    tmp = *x1; *x1 = *x9; *x9 = tmp;
    tmp = *x2; *x2 = *x8; *x8 = tmp;
    tmp = *x3; *x3 = *x7; *x7 = tmp;
    tmp = *x4; *x4 = *x6; *x6 = tmp;
}
