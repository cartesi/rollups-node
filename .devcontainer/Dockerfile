# syntax=docker/dockerfile:1
FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

ENV DEBIAN_FRONTEND=noninteractive

# [Optional] Uncomment this section to install additional OS packages.
RUN <<EOF
    apt-get update
    apt-get install -y --no-install-recommends \
        git \
        build-essential \
        libslirp-dev \
        libboost-all-dev \
        liblua5.4-dev \
        gcc-12-riscv64-linux-gnu \
        libc6-dev-riscv64-cross \
        wget \
        patch \
        && rm -rf /var/lib/apt/lists/*
EOF

# Clone and submodule update
RUN <<EOF
    git clone https://github.com/cartesi/machine-emulator-sdk.git --branch v0.19.0 --recurse-submodules
EOF

RUN <<EOF
    make -C machine-emulator-sdk toolchain
EOF

# [Optional] Uncomment the next lines to use go get to install anything else you need
# USER vscode
# RUN go get -x <your-dependency-or-tool>
# USER root

# [Optional] Uncomment this line to install global node packages.
# RUN su vscode -c "source /usr/local/share/nvm/nvm.sh && npm install -g <your-package-here>" 2>&1
