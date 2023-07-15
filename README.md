# GoAT

Go assembly transpiler for C programming languages.

It help to utilize optimization from C compiler in Go projects. For example, generate SIMD vectorized functions for Go (refer to How to Use AVX512 in Golang).

## Install

1. Install prerequisites

```bash
sudo apt update
sudo apt install clang libc6-dev-i386
```

2. Install GoAT

```bash
go install github.com/gorse-io/goat@latest
```

## Usage

```bash
cd example

goat src/mul_to.c -O3 -mavx -mfma -mavx512f -mavx512dq
GoAT transpiles example/src/mul_to.c to two files.
```

Go function definition file mul_to.go:

```go
//go:build !noasm && amd64
// AUTO-GENERATED BY GOAT -- DO NOT EDIT

package example

import "unsafe"

//go:noescape
func mul_to(a, b, c, n unsafe.Pointer)
Go assembly file mul_to.s:
//go:build !noasm && amd64
// AUTO-GENERATED BY GOAT -- DO NOT EDIT

TEXT ·mul_to(SB), $0-32
	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI
	MOVQ c+16(FP), DX
	MOVQ n+24(FP), CX
	BYTE $0x55               // pushq	%rbp
	WORD $0x8948; BYTE $0xe5 // movq	%rsp, %rbp
	LONG $0xf8e48348         // andq	$-8, %rsp
	WORD $0x8548; BYTE $0xc9 // testq	%rcx, %rcx
	JLE  LBB0_12
	LONG $0x3ff98348         // cmpq	$63, %rcx
	JA   LBB0_7
	WORD $0xc031             // xorl	%eax, %eax
	JMP  LBB0_3

LBB0_7:
	LONG $0x8a0c8d4c         // leaq	(%rdx,%rcx,4), %r9
	LONG $0x8f048d48         // leaq	(%rdi,%rcx,4), %rax
	WORD $0x3948; BYTE $0xd0 // cmpq	%rdx, %rax
	LONG $0xc2970f41         // seta	%r10b
	LONG $0x8e048d48         // leaq	(%rsi,%rcx,4), %rax
	WORD $0x3949; BYTE $0xf9 // cmpq	%rdi, %r9
	LONG $0xc3970f41         // seta	%r11b
	WORD $0x3948; BYTE $0xd0 // cmpq	%rdx, %rax
	LONG $0xc0970f41         // seta	%r8b
	WORD $0x3949; BYTE $0xf1 // cmpq	%rsi, %r9
	LONG $0xc1970f41         // seta	%r9b
	WORD $0xc031             // xorl	%eax, %eax
	WORD $0x8445; BYTE $0xda // testb	%r11b, %r10b
	JNE  LBB0_3
	WORD $0x2045; BYTE $0xc8 // andb	%r9b, %r8b
	JNE  LBB0_3
	WORD $0x8948; BYTE $0xc8 // movq	%rcx, %rax
	LONG $0xc0e08348         // andq	$-64, %rax
	WORD $0x3145; BYTE $0xc0 // xorl	%r8d, %r8d

LBB0_10:
	LONG $0x487cb162; WORD $0x0410; BYTE $0x87 // vmovups	(%rdi,%r8,4), %zmm0
	QUAD $0x01874c10487cb162                   // vmovups	64(%rdi,%r8,4), %zmm1
	QUAD $0x02875410487cb162                   // vmovups	128(%rdi,%r8,4), %zmm2
	QUAD $0x03875c10487cb162                   // vmovups	192(%rdi,%r8,4), %zmm3
	LONG $0x487cb162; WORD $0x0459; BYTE $0x86 // vmulps	(%rsi,%r8,4), %zmm0, %zmm0
	QUAD $0x01864c594874b162                   // vmulps	64(%rsi,%r8,4), %zmm1, %zmm1
	QUAD $0x02865459486cb162                   // vmulps	128(%rsi,%r8,4), %zmm2, %zmm2
	QUAD $0x03865c594864b162                   // vmulps	192(%rsi,%r8,4), %zmm3, %zmm3
	LONG $0x487cb162; WORD $0x0411; BYTE $0x82 // vmovups	%zmm0, (%rdx,%r8,4)
	QUAD $0x01824c11487cb162                   // vmovups	%zmm1, 64(%rdx,%r8,4)
	QUAD $0x02825411487cb162                   // vmovups	%zmm2, 128(%rdx,%r8,4)
	QUAD $0x03825c11487cb162                   // vmovups	%zmm3, 192(%rdx,%r8,4)
	LONG $0x40c08349                           // addq	$64, %r8
	WORD $0x394c; BYTE $0xc0                   // cmpq	%r8, %rax
	JNE  LBB0_10
	WORD $0x3948; BYTE $0xc8                   // cmpq	%rcx, %rax
	JE   LBB0_12

LBB0_3:
	WORD $0x8949; BYTE $0xc0 // movq	%rax, %r8
	WORD $0xf749; BYTE $0xd0 // notq	%r8
	WORD $0x0149; BYTE $0xc8 // addq	%rcx, %r8
	WORD $0x8949; BYTE $0xc9 // movq	%rcx, %r9
	LONG $0x03e18349         // andq	$3, %r9
	JE   LBB0_5

LBB0_4:
	LONG $0x0410fac5; BYTE $0x87 // vmovss	(%rdi,%rax,4), %xmm0
	LONG $0x0459fac5; BYTE $0x86 // vmulss	(%rsi,%rax,4), %xmm0, %xmm0
	LONG $0x0411fac5; BYTE $0x82 // vmovss	%xmm0, (%rdx,%rax,4)
	LONG $0x01c08348             // addq	$1, %rax
	LONG $0xffc18349             // addq	$-1, %r9
	JNE  LBB0_4

LBB0_5:
	LONG $0x03f88349 // cmpq	$3, %r8
	JB   LBB0_12

LBB0_6:
	LONG $0x0410fac5; BYTE $0x87   // vmovss	(%rdi,%rax,4), %xmm0
	LONG $0x0459fac5; BYTE $0x86   // vmulss	(%rsi,%rax,4), %xmm0, %xmm0
	LONG $0x0411fac5; BYTE $0x82   // vmovss	%xmm0, (%rdx,%rax,4)
	LONG $0x4410fac5; WORD $0x0487 // vmovss	4(%rdi,%rax,4), %xmm0
	LONG $0x4459fac5; WORD $0x0486 // vmulss	4(%rsi,%rax,4), %xmm0, %xmm0
	LONG $0x4411fac5; WORD $0x0482 // vmovss	%xmm0, 4(%rdx,%rax,4)
	LONG $0x4410fac5; WORD $0x0887 // vmovss	8(%rdi,%rax,4), %xmm0
	LONG $0x4459fac5; WORD $0x0886 // vmulss	8(%rsi,%rax,4), %xmm0, %xmm0
	LONG $0x4411fac5; WORD $0x0882 // vmovss	%xmm0, 8(%rdx,%rax,4)
	LONG $0x4410fac5; WORD $0x0c87 // vmovss	12(%rdi,%rax,4), %xmm0
	LONG $0x4459fac5; WORD $0x0c86 // vmulss	12(%rsi,%rax,4), %xmm0, %xmm0
	LONG $0x4411fac5; WORD $0x0c82 // vmovss	%xmm0, 12(%rdx,%rax,4)
	LONG $0x04c08348               // addq	$4, %rax
	WORD $0x3948; BYTE $0xc1       // cmpq	%rax, %rcx
	JNE  LBB0_6

LBB0_12:
	WORD $0x8948; BYTE $0xec // movq	%rbp, %rsp
	BYTE $0x5d               // popq	%rbp
	WORD $0xf8c5; BYTE $0x77 // vzeroupper
	BYTE $0xc3               // retq
Finally, the mul_to function can be called by:

func MulTo(a, b, c []float32) {
	if len(a) ! = len(b) || len(a) ! = len(c) {
		panic("floats: slice lengths do not match")
	}
	mul_to(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0]), unsafe.Pointer(&c[0]), unsafe.Pointer(uintptr(len(a))))
}
```

## Limitations

- Arguments need (for now) to be 64-bit size, meaning either a value or a pointer
- Maximum number of 4 arguments
- Generally no call statements

## Acknowledgments

GoAT is inspired by [c2goasm](https://github.com/minio/c2goasm).
