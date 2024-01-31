# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

# syntax=docker.io/docker/dockerfile:1

# This dockerfile contains multiple stages to build three final targets.
# The file was split in a section for each final target.

# Version config that should be set in the bake file.
ARG BASE_IMAGE
ARG RUST_VERSION
ARG GO_VERSION
ARG FOUNDRY_COMMIT_VERSION
ARG SERVER_MANAGER_VERSION
ARG MACHINE_EMULATOR_VERSION
ARG ROOTFS_VERSION
ARG LINUX_VERSION
ARG LINUX_KERNEL_VERSION
ARG ROM_VERSION

# Build directories.
ARG SNAPSHOT_BUILD_PATH=/build/snapshot
ARG DEVNET_BUILD_PATH=/build/devnet
ARG RUST_BUILD_PATH=/build/rollups-node/rust
ARG GO_BUILD_PATH=/build/rollups-node/go

# Runtime dir for the cartesi-machine snapshot.
ARG SNAPSHOT_RUNTIME_PATH=/usr/share/cartesi/snapshot

####################################################################################################
# STAGE: emulator-base
#
# This stage creates a base-image with the Cartesi machine emulator.
# The result is used as the base for the snapshot and the node targets.
# We do this instead of using the cartesi/machine-emulator image to have control over the distro
# used by the base image.
FROM ${BASE_IMAGE} as emulator-base

# Install machine-emulator
ARG MACHINE_EMULATOR_VERSION
RUN <<EOF
set -e
apt-get update
apt-get install -y --no-install-recommends \
    ca-certificates \
    curl
ARCH=$(dpkg --print-architecture)
curl -fsSL https://github.com/cartesi/machine-emulator/releases/download/v${MACHINE_EMULATOR_VERSION}/cartesi-machine-v${MACHINE_EMULATOR_VERSION}_${ARCH}.deb -o ./cartesi-machine.deb
apt-get install -y ./cartesi-machine.deb
rm ./cartesi-machine.deb
rm -rf /var/lib/apt/lists/*
EOF

# Configure cartesi user and group.
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

####################################################################################################
# TARGET: rollups-node-snapshot
#
# This target contains the Cartesi machine snapshot used in the rollups-node integration and
# end-to-end tests.

# STAGE: snapshot-builder
#
# This stage builds the snapshot using the machine emulator as base image.
FROM emulator-base as snapshot-builder

# Download rootfs, linux and rom.
# Add these files to the directories the cartesi-machine expects.
WORKDIR /usr/share/cartesi-machine/images/
ARG ROOTFS_VERSION
ARG LINUX_VERSION
ARG LINUX_KERNEL_VERSION
ARG ROM_VERSION
ADD https://github.com/cartesi/image-rootfs/releases/download/v${ROOTFS_VERSION}/rootfs-v${ROOTFS_VERSION}.ext2 rootfs.ext2
ADD https://github.com/cartesi/image-kernel/releases/download/v${LINUX_VERSION}/linux-${LINUX_KERNEL_VERSION}.bin linux.bin
ADD https://github.com/cartesi/machine-emulator-rom/releases/download/v${ROM_VERSION}/rom-v${ROM_VERSION}.bin rom.bin

# Generate snapshot with echo and store it.
ARG SNAPSHOT_BUILD_PATH
WORKDIR /build
RUN cartesi-machine \
    --ram-length=128Mi \
    --rollup \
    --store=$SNAPSHOT_BUILD_PATH \
    -- "ioctl-echo-loop --vouchers=1 --notices=1 --reports=1 --verbose=1"

# STAGE: rollups-node-snapshot
#
# This stage copies the image from the builder.
# We use the emulator as base image so we can easily create a container with a volume shared with
# the rollups-node container.
FROM emulator-base as rollups-node-snapshot

# Copy image from the builder stage.
ARG SNAPSHOT_BUILD_PATH
ARG SNAPSHOT_RUNTIME_PATH
WORKDIR ${SNAPSHOT_RUNTIME_PATH}
COPY --from=snapshot-builder --chown=cartesi:cartesi ${SNAPSHOT_BUILD_PATH} ${SNAPSHOT_RUNTIME_PATH}

# Set dummy command.
CMD /bin/bash

####################################################################################################
# TARGET: rollups-node-devnet
#
# This target contains the Ethereum node that rollups node uses for testing.
# This target requires the machine-snapshot built in the snapshot-builder stage.

# STAGE: devnet-base
#
# This stage installs Foundry.
FROM ${BASE_IMAGE} as devnet-base

# Install system dependencies.
ARG DEBIAN_FRONTEND=noninteractive
RUN <<EOF
set -e
apt-get update
apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    jq \
    wget \
    xxd
EOF

# Install Foundry.
RUN curl -L https://foundry.paradigm.xyz | bash
ENV PATH="~/.foundry/bin:${PATH}"
ARG FOUNDRY_COMMIT_VERSION
RUN <<EOF
set -e
bash -c foundryup -C ${FOUNDRY_COMMIT_VERSION}
ln -s ~/.foundry/bin/anvil /usr/bin/anvil
EOF

# STAGE: devnet-builder
#
# This stage builds the devnet state that will be loaded in Anvil.
FROM devnet-base as devnet-builder
ARG DEVNET_BUILD_PATH
WORKDIR ${DEVNET_BUILD_PATH}

# Copy machine snapshot hash.
ARG SNAPSHOT_BUILD_PATH
COPY --from=snapshot-builder ${SNAPSHOT_BUILD_PATH}/hash hash.bin

# Generate Anvil state.
COPY scripts scripts
RUN ./scripts/devnet/gen-devnet.sh -v -t hash.bin

# STAGE: rollups-node-devnet
#
# This stage installs the runtime utilities and copies the anvil state from the builder stage.
FROM devnet-base as rollups-node-devnet

# Copy anvil state file
ARG DEVNET_BUILD_PATH
ENV ANVIL_STATE_PATH=/usr/share/devnet/anvil_state.json
COPY --from=devnet-builder ${DEVNET_BUILD_PATH}/anvil_state.json ${ANVIL_STATE_PATH}

# Copy healthcheck script.
COPY scripts/devnet/lib/anvil_net_listening.sh /usr/bin/anvil_net_listening.sh
HEALTHCHECK --interval=1s --timeout=1s --retries=5 CMD anvil_net_listening.sh

# Make Anvil bind to the external network interface.
ENV ANVIL_IP_ADDR="0.0.0.0"

# Start Anvil.
CMD anvil --block-time 1 --load-state $ANVIL_STATE_PATH

####################################################################################################
# TARGET: rollups-node
#
# This target is the final Docker image that will be used in the production environment.
# It installs all the binaries necessary to run the node in /usr/bin.

# STAGE: rust-chef
#
# This stage installs the cargo-chef tool.
FROM rust:${RUST_VERSION}-bookworm AS rust-chef
ARG RUST_BUILD_PATH
WORKDIR ${RUST_BUILD_PATH}
RUN <<EOF
set -e
rustup component add rustfmt
cargo install cargo-chef
EOF

# STAGE: rust-prepare
#
# This stage prepares the recipe with just the external dependencies.
FROM rust-chef as rust-prepare
COPY ./offchain/ .
RUN cargo chef prepare --recipe-path recipe.json

# STAGE: rust-builder
#
# This stage builds the Rust binaries.
# First it builds the external dependencies and then it builds the node binaries.
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
COPY ./offchain/ .
RUN cargo build --release

# STAGE: go-builder
#
# This stage builds the Go binaries.
# First it downloads the external dependencies and then it builds the binaries.
FROM golang:${GO_VERSION}-bookworm as go-builder
ARG GO_BUILD_PATH
WORKDIR ${GO_BUILD_PATH}

# Download external dependencies.
COPY go.mod .
COPY go.sum .
RUN go mod download

# Build application.
COPY . .
RUN go build -ldflags "-s -w" ./cmd/cartesi-rollups-node

# STAGE: server-manager
#
# This stage creates an alias for the server-manager image with a specific version.
FROM cartesi/server-manager:${SERVER_MANAGER_VERSION} as server-manager

# STAGE: rollups-node
#
# This stage copies the binaries from the previous stages.
FROM emulator-base as rollups-node

# Download system dependencies required in runtime.
ARG DEBIAN_FRONTEND=noninteractive
RUN <<EOF
set -e
apt-get update
apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    libpq5 \
    redis \
    libboost-log1.74.0 \
    redis-tools
rm -rf /var/lib/apt/lists/*
EOF

# Copy server-manager.
COPY --from=server-manager /usr/bin/server-manager /usr/bin

# Copy Rust binaries.
# Explicitly copy each binary to avoid adding unnecessary files to the runtime image.
ARG RUST_BUILD_PATH
ARG RUST_TARGET=${RUST_BUILD_PATH}/target/release
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-advance-runner /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-authority-claimer /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-dispatcher /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-graphql-server /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-host-runner /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-indexer /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-inspect-server /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-state-server /usr/bin

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
