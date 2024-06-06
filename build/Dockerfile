# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

# syntax=docker.io/docker/dockerfile:1

# This dockerfile contains multiple stages to build three final targets.

# Version configuration that should be set in the bake file.
ARG BASE_IMAGE
ARG RUST_VERSION
ARG GO_VERSION
ARG FOUNDRY_NIGHTLY_VERSION
ARG ROM_VERSION
ARG ROLLUPS_NODE_VERSION
ARG MACHINE_EMULATOR_VERSION
ARG MACHINE_TOOLS_VERSION
ARG MACHINE_IMAGE_KERNEL_VERSION
ARG MACHINE_KERNEL_VERSION
ARG MACHINE_XGENEXT2FS_VERSION

# Build directories.
ARG SNAPSHOT_BUILD_PATH=/build/snapshot
ARG DEVNET_BUILD_PATH=/build/devnet
ARG RUST_BUILD_PATH=/build/rollups-node/rust
ARG GO_BUILD_PATH=/build/rollups-node/go

# Runtime directory for the cartesi-machine snapshot.
ARG SNAPSHOT_RUNTIME_PATH=/usr/share/cartesi/snapshot

# =============================================================================
# STAGE: emulator
#
# - Install ca-certificates and curl (setup).
# - Install the machine-emulator.
# - Download linux.bin.
# - Download rootfs.ext2.
#
# NOTE: We do not use the cartesi/machine-emulator image to have control over
# the distro used by the base image.
# =============================================================================

FROM ${BASE_IMAGE} as emulator

ARG MACHINE_EMULATOR_VERSION
ARG MACHINE_TOOLS_VERSION
ARG MACHINE_IMAGE_KERNEL_VERSION
ARG MACHINE_KERNEL_VERSION
ARG DEBIAN_FRONTEND=noninteractive

# Install ca-certificates and curl (setup).
RUN <<EOF
    set -e
    apt-get update
    apt-get install -y --no-install-recommends ca-certificates curl
EOF

# Install the cartesi-machine (cartesi/machine-emulator).
RUN <<EOF
    set -e
    ARCH=$(dpkg --print-architecture)
    URL=https://github.com/cartesi/machine-emulator/releases/download
    VERSION=v${MACHINE_EMULATOR_VERSION}
    ARTIFACT=cartesi-machine-${VERSION}_${ARCH}.deb
    curl -fsSL ${URL}/${VERSION}/${ARTIFACT} -o ./cartesi-machine.deb
    apt-get install -y ./cartesi-machine.deb
    rm ./cartesi-machine.deb
EOF

# Configure cartesi user and cartesi group.
RUN <<EOF
    set -e
    addgroup --system --gid 102 cartesi
    adduser --system --uid 102 \
        --disabled-login \
        --gecos "cartesi user" \
        --home /nonexistent \
        --ingroup cartesi \
        --no-create-home \
        --shell /bin/false \
        cartesi
EOF

# Download linux.bin (cartesi/image-kernel).
RUN <<EOF
    set -e
    URL=https://github.com/cartesi/image-kernel/releases/download
    VERSION=v${MACHINE_IMAGE_KERNEL_VERSION}
    ARTIFACT=linux-${MACHINE_KERNEL_VERSION}-ctsi-1-${VERSION}.bin
    curl -fsSL ${URL}/${VERSION}/${ARTIFACT} -o ./linux.bin
    mv ./linux.bin /usr/share/cartesi-machine/images
EOF

# Download rootfs.ext2 (cartesi/machine-emulator-tools).
RUN <<EOF
    set -e
    URL=https://github.com/cartesi/machine-emulator-tools/releases/download
    VERSION=v${MACHINE_TOOLS_VERSION}
    ARTIFACT=rootfs-tools-${VERSION}.ext2
    curl -fsSL ${URL}/${VERSION}/${ARTIFACT} -o ./rootfs.ext2
    mv ./rootfs.ext2 /usr/share/cartesi-machine/images
EOF

# Clean up.
RUN <<EOF
    rm -rf /var/lib/apt/lists/*
EOF

# =============================================================================
# STAGE: emulator-devel
#
# - Install libarchive13 (setup -- required by xgenext2fs).
# - Install libcmt.
# - Install xgenext2fs.
#
# IMPORTANT: Only works with the linux/amd64 platform!
#            (libarchive13 is amd64 exclusive.)
# =============================================================================

FROM emulator as emulator-devel

ARG MACHINE_TOOLS_VERSION
ARG MACHINE_XGENEXT2FS_VERSION
ARG DEBIAN_FRONTEND=noninteractive

# Install libarchive13 (setup -- required by xgenext2fs).
RUN <<EOF
    set -e
    apt-get update
    apt-get install -y --no-install-recommends libarchive13
EOF

# Install libcmt (cartesi/machine-emulator-tools).
RUN <<EOF
    set -e
    URL=https://github.com/cartesi/machine-emulator-tools/releases/download
    VERSION=v${MACHINE_TOOLS_VERSION}
    ARTIFACT=libcmt-${VERSION}-dev.deb
    curl -fsSL ${URL}/${VERSION}/${ARTIFACT} -o ./libcmt.deb
    apt-get install -y ./libcmt.deb
    rm ./libcmt.deb
    mv /usr/riscv64-linux-gnu/include/libcmt /usr/include/
    mv /usr/riscv64-linux-gnu/lib/libcmt.a /usr/lib/
    rm -rf /usr/riscv64-linux-gnu
EOF

# Install xgenext2fs (cartesi/genext2fs).
RUN <<EOF
    set -e
    URL=https://github.com/cartesi/genext2fs/releases/download
    VERSION=v${MACHINE_XGENEXT2FS_VERSION}
    ARTIFACT=xgenext2fs_amd64.deb
    curl -fsSL ${URL}/${VERSION}/${ARTIFACT} -o ./xgenext2fs.deb
    apt-get install -y ./xgenext2fs.deb
    rm ./xgenext2fs.deb
EOF

# Clean up.
RUN <<EOF
    rm -rf /var/lib/apt/lists/*
EOF

# =============================================================================
# STAGE: rollups-node-ci
#
# This stage prepares the CI image.
#
# - Install git and build-essential (setup).
# - Set CGO_ENABLED.
# - Install docker.
# =============================================================================

FROM emulator-devel as rollups-node-ci

# Install git and build-essential (setup).
RUN <<EOF
    set -e
    apt-get update
    apt-get install -y --no-install-recommends git-all build-essential
EOF

ENV CGO_ENABLED=1

# Install docker.
RUN <<EOF
    install -m 0755 -d /etc/apt/keyrings
    URL=https://download.docker.com/linux/debian
    curl -fsSL ${URL}/gpg -o /etc/apt/keyrings/docker.asc
    chmod a+r /etc/apt/keyrings/docker.asc
    ARCH=$(dpkg --print-architecture)
    SIGNED_BY=/etc/apt/keyrings/docker.asc
    echo "deb [arch=${ARCH} signed-by=${SIGNED_BY}] ${URL} $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
        | tee /etc/apt/sources.list.d/docker.list > /dev/null
    apt-get update
    apt-get install -y --no-install-recommends \
        docker-ce \
        docker-ce-cli \
        containerd.io \
        docker-buildx-plugin \
        docker-compose-plugin
    # Cartesi Machine Emulator
    SDK_URL=https://github.com/cartesi/machine-emulator-sdk
EOF

# =============================================================================
# STAGE: snapshot-builder
#
# - Build an echo snapshot.
#
# DEPRECATED: this stage is going to be deleted, as the CI won't be generating
# machine snapshots in the future.
# =============================================================================

FROM emulator as snapshot-builder

WORKDIR /build
ARG SNAPSHOT_BUILD_PATH
RUN cartesi-machine \
    --ram-length=128Mi \
    --store=$SNAPSHOT_BUILD_PATH \
    --no-rollback \
    -- "ioctl-echo-loop --vouchers=1 --notices=1 --reports=1 --verbose=1"

# =============================================================================
# STAGE: rollups-node-snapshot
#
# DEPRECATED: this stage is going to be deleted, as the CI won't be generating
# machine snapshots in the future.
# =============================================================================

FROM emulator as rollups-node-snapshot

# Copy image from the builder stage.
ARG SNAPSHOT_BUILD_PATH
ARG SNAPSHOT_RUNTIME_PATH
WORKDIR ${SNAPSHOT_RUNTIME_PATH}
COPY --from=snapshot-builder --chown=cartesi:cartesi \
    ${SNAPSHOT_BUILD_PATH} ${SNAPSHOT_RUNTIME_PATH}

# Set dummy command.
CMD /bin/bash

# =============================================================================
# STAGE: devnet-base
#
# - Install ca-certificates, curl, and git (setup).
# - Install Foundry from downloaded pre-compiled binaries.
# =============================================================================

FROM ${BASE_IMAGE} as devnet-base

# Install ca-certificates, curl, and git (setup).
ARG DEBIAN_FRONTEND=noninteractive
RUN <<EOF
    set -e
    apt-get update
    apt-get install -y --no-install-recommends ca-certificates curl git
EOF

# Install Foundry from downloaded pre-compiled binaries.
ARG FOUNDRY_NIGHTLY_VERSION
RUN <<EOF
    set -e
    URL=https://github.com/foundry-rs/foundry/releases/download
    VERSION=nightly-${FOUNDRY_NIGHTLY_VERSION}
    ARCH=$(dpkg --print-architecture)
    ARTIFACT=foundry_nightly_linux_${ARCH}.tar.gz
    curl -sSL ${URL}/${VERSION}/${ARTIFACT} | tar -zx -C /usr/local/bin
EOF

# =============================================================================
# STAGE: devnet-builder
#
# This stage builds the devnet state that will be loaded in Anvil.
# =============================================================================

FROM devnet-base as devnet-builder
ARG DEVNET_BUILD_PATH
WORKDIR ${DEVNET_BUILD_PATH}

# Install gnupg (setup).
ARG DEBIAN_FRONTEND=noninteractive
RUN <<EOF
    set -e
    apt-get update
    apt-get install -y --no-install-recommends gnupg
EOF

# Install nodejs.
ARG DEBIAN_FRONTEND=noninteractive
RUN <<EOF
    set -e
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
    apt-get install -y nodejs
EOF

# Install pnpm.
RUN npm install -g @pnpm/exe

# Install Go.
ARG GO_VERSION
RUN <<EOF
    set -e
    ARCH=$(dpkg --print-architecture)
    URL=https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz
    curl -sSL ${URL} | tar -zx -C /usr/local
EOF
ENV PATH=$PATH:/usr/local/go/bin

# Copy gen-devnet script and dependencies.
# NOTE: this should be simplified in the future when
#       `COPY --parents` is made available.
COPY internal internal
COPY pkg pkg
COPY rollups-contracts rollups-contracts
COPY cmd/gen-devnet cmd/gen-devnet

# Download external dependencies.
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy machine snapshot hash.
ARG SNAPSHOT_BUILD_PATH
COPY --from=snapshot-builder ${SNAPSHOT_BUILD_PATH}/hash hash.bin

# Generate anvil state.
RUN go run ./cmd/gen-devnet --verbose --template-hash-file hash.bin

# =============================================================================
# STAGE: rollups-node-devnet
#
# This stage contains the Ethereum node that the rollups node uses for testing.
#
# It copies the anvil state from the builder stage and starts the local anvil
# instance.
# =============================================================================

FROM devnet-base as rollups-node-devnet

# Copy anvil state file and devnet deployment info.
ARG DEVNET_BUILD_PATH
ARG DEVNET_SHARE_PATH=/usr/share/devnet
ENV ANVIL_STATE_PATH=${DEVNET_SHARE_PATH}/anvil_state.json
ENV DEPLOYMENT_JSON_PATH=${DEVNET_SHARE_PATH}/deployment.json
COPY --from=devnet-builder ${DEVNET_BUILD_PATH}/anvil_state.json ${ANVIL_STATE_PATH}
COPY --from=devnet-builder ${DEVNET_BUILD_PATH}/deployment.json ${DEPLOYMENT_JSON_PATH}

# Make Anvil bind to the external network interface.
ENV ANVIL_IP_ADDR="0.0.0.0"
ENV RPC_URL="http://${ANVIL_IP_ADDR}:8545"
HEALTHCHECK --interval=1s --timeout=1s --retries=5 \
    CMD curl \
        -X \
        POST \
        -s \
        -H 'Content-Type: application/json' \
        -d '{"jsonrpc":"2.0","id":"1","method":"net_listening","params":[]}' \
        ${RPC_URL}

# Start Anvil.
CMD anvil --block-time 1 --load-state $ANVIL_STATE_PATH

# =============================================================================
# STAGE: rust-chef
#
# - Instal cargo-chef.
# =============================================================================

FROM rust:${RUST_VERSION}-bookworm AS rust-chef
ARG RUST_BUILD_PATH
WORKDIR ${RUST_BUILD_PATH}
RUN <<EOF
    set -e
    rustup component add rustfmt
    cargo install cargo-chef
EOF

# =============================================================================
# STAGE: rust-prepare
#
# This stage prepares the recipe with just the external dependencies.
# =============================================================================

FROM rust-chef as rust-prepare
COPY ./cmd/authority-claimer/ .
RUN cargo chef prepare --recipe-path recipe.json

# =============================================================================
# STAGE: rust-builder
#
# This stage builds the Rust binaries. First it builds the external
# dependencies and then it builds the node binaries.
# =============================================================================

FROM rust-chef as rust-builder

# Install system dependencies.
ARG DEBIAN_FRONTEND=noninteractive
RUN <<EOF
    set -e
    apt-get update
    apt-get install -y --no-install-recommends \
        cmake \
        curl \
        libprotobuf-dev \
        protobuf-compiler
EOF

# Build external dependencies with cargo chef.
COPY --from=rust-prepare ${RUST_BUILD_PATH}/recipe.json .
RUN cargo chef cook --release --recipe-path recipe.json

# Build application.
COPY ./cmd/authority-claimer/ .
RUN cargo build --release

# =============================================================================
# STAGE: go-builder
#
# This stage builds the Go binaries. First it downloads the external
# dependencies and then it builds the binaries.
# =============================================================================

FROM golang:${GO_VERSION}-bookworm as go-builder
ARG GO_BUILD_PATH
ARG ROLLUPS_NODE_VERSION
WORKDIR ${GO_BUILD_PATH}

# Download external dependencies.
COPY go.mod .
COPY go.sum .
RUN go mod download

# Build application.
COPY . .
RUN go build -ldflags "-s -w -X 'main.buildVersion=${ROLLUPS_NODE_VERSION}'" ./cmd/cartesi-rollups-node

# =============================================================================
# STAGE: rollups-node
#
# This stage prepares the final Docker image that will be used in the production
# environment. It installs in /usr/bin all the binaries necessary to run the
# node.
#
# (This stage copies the binaries from previous stages.)
# =============================================================================

FROM emulator as rollups-node

# Download system dependencies required at runtime.
ARG DEBIAN_FRONTEND=noninteractive
RUN <<EOF
    set -e
    apt-get update
    apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        libpq5 \
        redis \
        redis-tools \
        libboost-coroutine1.81.0 \
        libboost-context1.81.0 \
        libboost-filesystem1.81.0 \
        libboost-log1.81.0 \
        libcrypto++8 \
        procps \
        nodejs \
        npm
    rm -rf /var/lib/apt/lists/*
    npm install -g postgraphile
    npm install -g --save @graphile-contrib/pg-simplify-inflector
EOF

# Copy Rust binaries.
# Explicitly copy each binary to avoid adding unnecessary files to the runtime
# image.
ARG RUST_BUILD_PATH
ARG RUST_TARGET=${RUST_BUILD_PATH}/target/release
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-authority-claimer /usr/bin

# Copy Go binary.
ARG GO_BUILD_PATH
COPY --from=go-builder ${GO_BUILD_PATH}/cartesi-rollups-node /usr/bin

# Setup runtime dir.
ARG RUNTIME_DIR=/usr/share/rollups-node
WORKDIR ${RUNTIME_DIR}
RUN chown cartesi:cartesi ${RUNTIME_DIR}

# Set user to low-privilege.
USER cartesi

# Set the Go supervisor as the command.
CMD [ "cartesi-rollups-node" ]
