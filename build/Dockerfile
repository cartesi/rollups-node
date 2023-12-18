# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

# syntax=docker.io/docker/dockerfile:1.4

ARG RUST_VERSION=1.74.0
ARG GO_VERSION=1.21.1
ARG SERVER_MANAGER_VERSION=0.8.2
ARG ROLLUPS_CONTRACTS_VERSION=1.1.0
ARG MACHINE_EMULATOR_VERSION=0.15.2

ARG BASE_PATH=/opt/cartesi
ARG RUST_BUILD_PATH=${BASE_PATH}/src/rollups-node/offchain
ARG GO_BASE_PATH=/go
ARG GO_BUILD_PATH=${GO_BASE_PATH}/cartesi-rollups-node
ARG GO_BIN_PATH=${GO_BASE_PATH}/bin
ARG RUNTIME_DIR=/var/opt/cartesi
ARG MACHINE_SNAPSHOT_PATH=/var/opt/cartesi/machine-snapshots
ARG DEVNET_SHARE_PATH=/usr/share/devnet
ARG ANVIL_IP_ADDR=0.0.0.0

#
# Machine snapshot generation
#
FROM cartesi/machine-emulator:${MACHINE_EMULATOR_VERSION} as machine-snapshot-build

USER root

# Download rootfs, linux and rom
WORKDIR /usr/share/cartesi-machine/images/
ADD https://github.com/cartesi/image-rootfs/releases/download/v0.18.0/rootfs-v0.18.0.ext2 rootfs.ext2
ADD https://github.com/cartesi/image-kernel/releases/download/v0.17.0/linux-5.15.63-ctsi-2-v0.17.0.bin linux.bin
ADD https://github.com/cartesi/machine-emulator-rom/releases/download/v0.17.0/rom-v0.17.0.bin rom.bin

# Generate snapshot with echo and store it
ENV SNAPSHOT_DIR=/tmp/dapp-bin
RUN cartesi-machine \
    --ram-length=128Mi \
    --rollup \
    --store=$SNAPSHOT_DIR \
    -- "ioctl-echo-loop --vouchers=1 --notices=1 --reports=1 --verbose=1"

FROM debian:bookworm-slim as machine-snapshot

RUN <<EOF
addgroup --system \--gid 102 cartesi
adduser --system --uid 102 --ingroup cartesi --disabled-login --no-create-home --home /nonexistent --gecos "cartesi user" --shell /bin/false cartesi
EOF

ARG MACHINE_SNAPSHOT_PATH
COPY --from=machine-snapshot-build --chown=cartesi:cartesi /tmp/dapp-bin ${MACHINE_SNAPSHOT_PATH}/0_0
RUN ln -s ${MACHINE_SNAPSHOT_PATH}/0_0 ${MACHINE_SNAPSHOT_PATH}/latest

WORKDIR ${MACHINE_SNAPSHOT_PATH}
CMD [ "/bin/bash" ]

#
# Devnet generation stage
#
FROM debian:bookworm-slim as rollups-devnet

ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    jq \
    wget \
    xxd

# Install foundry
RUN curl -L https://foundry.paradigm.xyz | bash
ENV PATH="~/.foundry/bin:${PATH}"
RUN bash -c foundryup
RUN ln -s ~/.foundry/bin/anvil /usr/local/bin/anvil

ARG RUNTIME_DIR
WORKDIR ${RUNTIME_DIR}
COPY scripts scripts

ARG DEVNET_SHARE_PATH
WORKDIR ${DEVNET_SHARE_PATH}

# Copy machine snapshot hash
ARG MACHINE_SNAPSHOT_PATH
COPY --from=machine-snapshot ${MACHINE_SNAPSHOT_PATH}/0_0/hash 0_0_hash

# Generate devnet
ENV ANVIL_IP_ADDR="${ANVIL_IP_ADDR}"
RUN ${RUNTIME_DIR}/scripts/devnet/gen-devnet.sh \
    -t 0_0_hash

# Install utilities
RUN ln -s ${RUNTIME_DIR}/scripts/devnet/lib/anvil_net_listening.sh \
    /usr/local/bin/anvil_net_listening.sh

HEALTHCHECK --interval=10s --timeout=1s --retries=5 \
  CMD anvil_net_listening.sh

ENTRYPOINT ["anvil"]
CMD ["--block-time", "5", "--load-state" , "/usr/share/devnet/anvil_state.json"]

#
# Cargo chef
#
FROM rust:${RUST_VERSION}-bookworm AS rust-chef

RUN <<EOF
rustup component add rustfmt
cargo install cargo-chef
EOF

# The workdir will be inherited by the following Rust images
ARG RUST_BUILD_PATH
WORKDIR ${RUST_BUILD_PATH}

#
# Cargo chef prepare stage
#
FROM rust-chef as rust-planner

COPY ./offchain/ .
RUN cargo chef prepare --recipe-path recipe.json

#
# Cargo chef cook stage
#
FROM rust-chef as rust-builder

ARG DEBIAN_FRONTEND=noninteractive
RUN <<EOF
apt-get update
apt-get install -y --no-install-recommends \
    cmake \
    curl \
    libprotobuf-dev \
    protobuf-compiler
EOF

# Build dependencies with cargo chef
COPY --from=rust-planner ${RUST_BUILD_PATH}/recipe.json .
RUN cargo chef cook --release --recipe-path recipe.json

# Build application
COPY ./offchain/ .
# needed to log commit hash in runtime
COPY ./.git ../
RUN cargo build --release

#
# Go build
#
FROM golang:${GO_VERSION}-bookworm as go-builder
ARG GO_BUILD_PATH
WORKDIR ${GO_BUILD_PATH}
COPY . .
RUN go install -ldflags "-s -w" ./cmd/cartesi-rollups-node

#
# Runtime
#
FROM cartesi/server-manager:${SERVER_MANAGER_VERSION} as rollups-node

USER root

ARG DEBIAN_FRONTEND=noninteractive
RUN <<EOF
apt-get update
apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    libpq5 \
    redis \
    redis-tools
rm -rf /var/lib/apt/lists/*
EOF

# Copy Rust binaries
ARG BASE_PATH
WORKDIR ${BASE_PATH}/bin
ARG RUST_BUILD_PATH
COPY --from=rust-builder ${RUST_BUILD_PATH}/target/release/cartesi-rollups-* .
ENV PATH="${BASE_PATH}/bin:${PATH}"

# Copy Go binary
ARG GO_BIN_PATH
COPY --from=go-builder ${GO_BIN_PATH}/cartesi-rollups-node .

# Setup runtime dir
ARG RUNTIME_DIR
RUN <<EOF
mkdir -p ${RUNTIME_DIR}
chown cartesi:cartesi ${RUNTIME_DIR}
EOF
WORKDIR ${RUNTIME_DIR}

USER cartesi
CMD ["cartesi-rollups-node"]
