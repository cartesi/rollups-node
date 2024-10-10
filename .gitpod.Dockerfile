# syntax=docker/dockerfile:1
FROM ubuntu:24.04

# Install:
# - git (and git-lfs), for git operations (to e.g. push your work).
#   Also required for setting up your configured dotfiles in the workspace.
# - sudo, while not required, is recommended to be installed, since the
#   workspace user (`gitpod`) is non-root and won't be able to install
#   and use `sudo` to install any other tools in a live workspace.
RUN <<EOF
apt-get update && apt-get install -yq \
    git \
    git-lfs \
    sudo \
    rustup \
    golang-1.21 \
    build-essential \
    libslirp-dev \
    libboost-all-dev \
    liblua5.4-dev \
    gcc-12-riscv64-linux-gnu \
    libc6-dev-riscv64-cross \
    wget \
    patch \
    && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/*
EOF

# Create the gitpod user. UID must be 33333.
RUN useradd -l -u 33333 -G sudo -md /home/gitpod -s /bin/bash -p gitpod gitpod

USER gitpod

RUN <<EOF
  rustup toolchain install 1.81.0-x86_64-unknown-linux-gnu
  rustup toolchain add 1.81.0-riscv64gc-unknown-linux-gnu
EOF
