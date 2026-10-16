FROM mcr.microsoft.com/devcontainers/go:2-1.27-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends clang binutils \
    && if [ "$(dpkg --print-architecture)" = "amd64" ]; then \
        apt-get install -y --no-install-recommends libc6-dev-i386; \
    fi \
    && rm -rf /var/lib/apt/lists/*
