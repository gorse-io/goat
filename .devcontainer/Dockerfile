FROM mcr.microsoft.com/devcontainers/go:2-1.27-trixie

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        clang lld binutils qemu-user-static \
        binutils-x86-64-linux-gnu gcc-x86-64-linux-gnu g++-x86-64-linux-gnu \
        binutils-aarch64-linux-gnu gcc-aarch64-linux-gnu g++-aarch64-linux-gnu \
        binutils-riscv64-linux-gnu gcc-riscv64-linux-gnu g++-riscv64-linux-gnu \
        binutils-loongarch64-linux-gnu gcc-loongarch64-linux-gnu g++-loongarch64-linux-gnu \
        binutils-s390x-linux-gnu gcc-s390x-linux-gnu g++-s390x-linux-gnu \
        binutils-powerpc64le-linux-gnu gcc-powerpc64le-linux-gnu g++-powerpc64le-linux-gnu \
    && if [ "$(dpkg --print-architecture)" = "amd64" ]; then \
        apt-get install -y --no-install-recommends libc6-dev-i386; \
    fi \
    && rm -rf /var/lib/apt/lists/*

# GoAT uses the Clang triple when locating the cross-architecture objdump.
RUN ln -s /usr/bin/x86_64-linux-gnu-objdump /usr/local/bin/amd64-linux-gnu-objdump
