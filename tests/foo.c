void foo(float *a, float *res)
{
    *res = bar(a);
}

int bar(float *a)
{
    return *a + 10;
}
