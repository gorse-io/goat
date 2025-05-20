long add(long a, long b) {
    return a + b;
}

float l2(float *a, float *b, long n)
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

long long sum(long long x1, long long x2, long long x3, long long x4, long long x5, long long x6, long long x7, long long x8)
{
    return x1 + x2 + x3 + x4 + x5 + x6 + x7 + x8;
}
