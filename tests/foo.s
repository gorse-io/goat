	.text
	.file	"foo.c"
	.section	.rodata.cst4,"aM",@progbits,4
	.p2align	2                               # -- Begin function foo
.LCPI0_0:
	.long	0x41200000                      # float 10
	.text
	.globl	foo
	.p2align	4, 0x90
	.type	foo,@function
foo:                                    # @foo
# %bb.0:
	pushq	%rbp
	movq	%rsp, %rbp
	andq	$-8, %rsp
	vmovss	(%rdi), %xmm0                   # xmm0 = mem[0],zero,zero,zero
	vaddss	.LCPI0_0(%rip), %xmm0, %xmm0
	vcvttps2dq	%xmm0, %xmm0
	vcvtdq2ps	%xmm0, %xmm0
	vmovss	%xmm0, (%rsi)
	movq	%rbp, %rsp
	popq	%rbp
	retq
.Lfunc_end0:
	.size	foo, .Lfunc_end0-foo
                                        # -- End function
	.section	.rodata.cst4,"aM",@progbits,4
	.p2align	2                               # -- Begin function bar
.LCPI1_0:
	.long	0x41200000                      # float 10
	.text
	.globl	bar
	.p2align	4, 0x90
	.type	bar,@function
bar:                                    # @bar
# %bb.0:
	pushq	%rbp
	movq	%rsp, %rbp
	andq	$-8, %rsp
	vmovss	(%rdi), %xmm0                   # xmm0 = mem[0],zero,zero,zero
	vaddss	.LCPI1_0(%rip), %xmm0, %xmm0
	vcvttss2si	%xmm0, %eax
	movq	%rbp, %rsp
	popq	%rbp
	retq
.Lfunc_end1:
	.size	bar, .Lfunc_end1-bar
                                        # -- End function
	.ident	"Ubuntu clang version 14.0.0-1ubuntu1"
	.section	".note.GNU-stack","",@progbits
	.addrsig
