void l2(float *a, float *b, float *res, long *len)
{
    int size = *len;

    float sum = 0;
    // add the remaining vectors
    for (int i = 0; i < size; i++)
    {
        float diff = a[i] - b[i];
        float sq = diff * diff;
        sum += sq;
    }

    *res = sum;
}