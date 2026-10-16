FROM mcr.microsoft.com/devcontainers/go:2-1.27-trixie

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        clang binutils qemu-user-static \
        binutils-x86-64-linux-gnu \
        binutils-aarch64-linux-gnu \
        binutils-riscv64-linux-gnu \
        binutils-loongarch64-linux-gnu \
        binutils-s390x-linux-gnu \
        binutils-powerpc64le-linux-gnu gcc-powerpc64le-linux-gnu \
    && if [ "$(dpkg --print-architecture)" = "amd64" ]; then \
        apt-get install -y --no-install-recommends libc6-dev-i386; \
    fi \
    && rm -rf /var/lib/apt/lists/*
