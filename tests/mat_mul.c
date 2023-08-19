void mat_mul(float *a, float *b, float *res, long d1, long d2, long d3)
{
    // add the remaining vectors
    for (int i = 0; i < d1; i++)
    {
        for (int j = 0; j < d3; j++) {
            float sum = 0;
            for (int k = 0; k < d2; k++) {
                sum += a[i * d2 + k] * b[k * d3 + j];
            }
            res[i * d3 + j] = sum;
        }
    }
}
