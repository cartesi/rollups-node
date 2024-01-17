# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

# syntax=docker.io/docker/dockerfile:1.4

# This dockerfile contains multiple stages to build three final targets.
# The file was split in a section for each final target.

ARG RUST_VERSION=1.75.0
ARG GO_VERSION=1.21.1
ARG FOUNDRY_VERSION_COMMIT=24abca6c9133618e0c355842d2be2dd4f36da46d
ARG ROLLUPS_CONTRACTS_VERSION=1.1.0
ARG SERVER_MANAGER_VERSION=0.8.2
ARG MACHINE_EMULATOR_VERSION=0.15.2

# Build directories.
ARG MACHINE_SNAPSHOT_BUILD_PATH=/build/machine-snapshot
ARG DEVNET_BUILD_PATH=/build/devnet
ARG RUST_BUILD_PATH=/build/rollups-node/rust
ARG GO_BUILD_PATH=/build/rollups-node/go

# Runtime dir for the cartesi-machine snapshot.
ARG MACHINE_SNAPSHOT_RUNTIME_PATH=/usr/share/cartesi/snapshots

####################################################################################################
# TARGET: rollups-node-snapshot
#
# This target contains the Cartesi machine snapshot used in the rollups-node integration and
# end-to-end tests.

# STAGE: snapshot-builder
#
# This stage builds the snapshot using the machine emulator as base image.
FROM cartesi/machine-emulator:${MACHINE_EMULATOR_VERSION} as snapshot-builder

# Set user to root because the base image uses a low-privilege user.
USER root

# Download rootfs, linux and rom.
# Add these files to the directories the cartesi-machine expects.
WORKDIR /usr/share/cartesi-machine/images/
ADD https://github.com/cartesi/image-rootfs/releases/download/v0.18.0/rootfs-v0.18.0.ext2 rootfs.ext2
ADD https://github.com/cartesi/image-kernel/releases/download/v0.17.0/linux-5.15.63-ctsi-2-v0.17.0.bin linux.bin
ADD https://github.com/cartesi/machine-emulator-rom/releases/download/v0.17.0/rom-v0.17.0.bin rom.bin

# Generate snapshot with echo and store it.
ARG MACHINE_SNAPSHOT_BUILD_PATH
WORKDIR ${MACHINE_SNAPSHOT_BUILD_PATH}
RUN cartesi-machine \
    --ram-length=128Mi \
    --rollup \
    --store=$MACHINE_SNAPSHOT_BUILD_PATH/snapshot \
    -- "ioctl-echo-loop --vouchers=1 --notices=1 --reports=1 --verbose=1"

# STAGE: rollups-node-snapshot
#
# This stage copies the image from the builder.
# We use the machine-emulator as base image so we can easily create a container with a volume shared
# with the rollups-node container.
FROM cartesi/machine-emulator:${MACHINE_EMULATOR_VERSION} as rollups-node-snapshot

# Copy image from the builder stage.
ARG MACHINE_SNAPSHOT_BUILD_PATH
ARG MACHINE_SNAPSHOT_RUNTIME_PATH
WORKDIR ${MACHINE_SNAPSHOT_RUNTIME_PATH}
COPY --from=snapshot-builder --chown=cartesi:cartesi ${MACHINE_SNAPSHOT_BUILD_PATH}/snapshot ./0_0
RUN ln -s ${MACHINE_SNAPSHOT_RUNTIME_PATH}/0_0 ${MACHINE_SNAPSHOT_RUNTIME_PATH}/latest

# Set dummy entrypoint
CMD /bin/bash

####################################################################################################
# TARGET: rollups-node-devnet
#
# This target contains the Ethereum node that rollups node uses for testing.
# This target requires the machine-snapshot built in the snapshot-builder stage.

# STAGE: devnet-base
#
# This stage installs Foundry.
FROM debian:bookworm-slim as devnet-base

# Install system dependencies.
ARG DEBIAN_FRONTEND=noninteractive
RUN <<EOF
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
ARG FOUNDRY_VERSION_COMMIT
RUN <<EOF
bash -c foundryup -C ${FOUNDRY_VERSION_COMMIT}
ln -s ~/.foundry/bin/anvil /usr/bin/anvil
EOF

# STAGE: devnet-builder
#
# This stage builds the devnet state that will be loaded in Anvil.
FROM devnet-base as devnet-builder
ARG DEVNET_BUILD_PATH
WORKDIR ${DEVNET_BUILD_PATH}

# Copy machine snapshot hash.
ARG MACHINE_SNAPSHOT_BUILD_PATH
COPY --from=snapshot-builder ${MACHINE_SNAPSHOT_BUILD_PATH}/snapshot/hash hash.bin

# Generate Anvil state.
COPY scripts scripts
RUN ./scripts/devnet/gen-devnet.sh -v -t hash.bin

# STAGE: rollups-node-devnet
#
# This stage install the runtime utilities and copies the anvil state from the builder stage.
FROM devnet-base as rollups-node-devnet

# Copy anvil state file
ARG DEVNET_BUILD_PATH
ENV ANVIL_STATE_PATH=/usr/share/devnet/anvil_state.json
COPY --from=devnet-builder ${DEVNET_BUILD_PATH}/anvil_state.json ${ANVIL_STATE_PATH}

# Copy healthcheck script.
COPY scripts/devnet/lib/anvil_net_listening.sh /usr/bin/anvil_net_listening.sh
HEALTHCHECK --interval=10s --timeout=1s --retries=5 CMD anvil_net_listening.sh

# Make Anvil bind to the external network interface.
ENV ANVIL_IP_ADDR="0.0.0.0"

# Start Anvil.
CMD anvil --block-time 5 --load-state $ANVIL_STATE_PATH

####################################################################################################
# TARGET: rollups-node
#
# This target is the final Docker image that will be used in the production environment.
# It install all the binaries necessary to run the node in /usr/bin.

# STAGE: rust-chef
#
# This stage installs the cargo-chef tool.
FROM rust:${RUST_VERSION}-bookworm AS rust-chef
ARG RUST_BUILD_PATH
WORKDIR ${RUST_BUILD_PATH}
RUN <<EOF
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

# STAGE: rollups-node
#
# This stage copies the binaries from the previous stages.
FROM cartesi/server-manager:${SERVER_MANAGER_VERSION} as rollups-node

# Set user to root because the base image uses a low-privilege user.
USER root

# Download system dependencies required in runtime.
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

# Set user back to low-privilege.
USER cartesi

# Set the Go supervisor as the entrypoint.
CMD cartesi-rollups-node
