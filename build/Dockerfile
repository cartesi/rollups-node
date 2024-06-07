# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

# syntax=docker.io/docker/dockerfile:1

# This dockerfile contains multiple stages to build three final targets.
# The file was split in a section for each final target.

# Version config that should be set in the bake file.
ARG BASE_IMAGE
ARG RUST_VERSION
ARG GO_VERSION
ARG FOUNDRY_NIGHTLY_VERSION
ARG SERVER_MANAGER_VERSION
ARG MACHINE_EMULATOR_VERSION
ARG ROOTFS_VERSION
ARG LINUX_VERSION
ARG LINUX_KERNEL_VERSION
ARG ROM_VERSION
ARG ROLLUPS_NODE_VERSION

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
ARG DEBIAN_FRONTEND=noninteractive
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

# Download rootfs and linux.
# Add these files to the directories the cartesi-machine expects.
WORKDIR /usr/share/cartesi-machine/images/
ARG TOOLS_VERSION
ARG LINUX_VERSION
ARG LINUX_KERNEL_VERSION
ADD https://github.com/cartesi/machine-emulator-tools/releases/download/v${TOOLS_VERSION}/rootfs-tools-v${TOOLS_VERSION}.ext2 rootfs.ext2
ADD https://github.com/cartesi/image-kernel/releases/download/v${LINUX_VERSION}/linux-${LINUX_KERNEL_VERSION}.bin linux.bin

# Generate snapshot with echo and store it.
WORKDIR /build
ARG SNAPSHOT_BUILD_PATH
RUN cartesi-machine \
    --ram-length=128Mi \
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
    git
EOF

# Install Foundry from downloaded pre-compiled binaries
ARG FOUNDRY_NIGHTLY_VERSION
RUN curl -sSL https://github.com/foundry-rs/foundry/releases/download/nightly-${FOUNDRY_NIGHTLY_VERSION}/foundry_nightly_linux_$(dpkg --print-architecture).tar.gz | \
        tar -zx -C /usr/local/bin

# STAGE: devnet-builder
#
# This stage builds the devnet state that will be loaded in Anvil.
FROM devnet-base as devnet-builder
ARG DEVNET_BUILD_PATH
WORKDIR ${DEVNET_BUILD_PATH}

# Install system dependencies.
ARG DEBIAN_FRONTEND=noninteractive
RUN <<EOF
set -e
apt-get update
apt-get install -y --no-install-recommends \
    gnupg
EOF

# Install yarn
ARG DEBIAN_FRONTEND=noninteractive
RUN <<EOF
set -e
curl -sS https://dl.yarnpkg.com/debian/pubkey.gpg | apt-key add -
echo "deb https://dl.yarnpkg.com/debian/ stable main" | tee /etc/apt/sources.list.d/yarn.list
apt-get update
apt-get install -y --no-install-recommends \
    nodejs \
    yarn
EOF

# Install Go
ARG GO_VERSION
RUN curl -sSL https://go.dev/dl/go${GO_VERSION}.linux-$(dpkg --print-architecture).tar.gz | \
         tar -zx -C /usr/local
ENV PATH=$PATH:/usr/local/go/bin

# Copy gen-devnet script and dependencies
# This should be simplified in the future when `COPY --parents` is made available
COPY internal internal
COPY pkg pkg
COPY rollups-contracts rollups-contracts
COPY cmd/gen-devnet cmd/gen-devnet

# Download external dependencies
COPY go.mod .
COPY go.sum .
RUN go mod download

# Copy machine snapshot hash
ARG SNAPSHOT_BUILD_PATH
COPY --from=snapshot-builder ${SNAPSHOT_BUILD_PATH}/hash hash.bin

# Generate anvil state
RUN go run ./cmd/gen-devnet --verbose --template-hash-file hash.bin

# STAGE: rollups-node-devnet
#
# This stage copies the anvil state from the builder stage and start the local anvil instance.
FROM devnet-base as rollups-node-devnet

# Copy anvil state file and devnet deployment info
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
ARG ROLLUPS_NODE_VERSION
WORKDIR ${GO_BUILD_PATH}

# Download external dependencies.
COPY go.mod .
COPY go.sum .
RUN go mod download

# Build application.
COPY . .
RUN go build -ldflags "-s -w -X 'main.buildVersion=${ROLLUPS_NODE_VERSION}'" ./cmd/cartesi-rollups-node

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
    redis-tools \
    libboost-coroutine1.81.0 \
    libboost-context1.81.0 \
    libboost-filesystem1.81.0 \
    libboost-log1.81.0 \
    libcrypto++8 \
    procps
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


####################################################################################################
# TARGET: rollups-node-distroless
#
# This target will build a rollups-node:$TAG-distroless image

# STAGE: distroless-amd64
#
# This stage copies the required libraries for amd64 (x86_64).
#
FROM scratch AS distroless-amd64
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_bad_optional_access.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_bad_optional_access.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_bad_variant_access.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_bad_variant_access.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_base.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_base.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_city.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_city.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_cord_internal.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_cord_internal.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_cord.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_cord.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_cordz_functions.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_cordz_functions.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_cordz_handle.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_cordz_handle.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_cordz_info.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_cordz_info.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_debugging_internal.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_debugging_internal.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_demangle_internal.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_demangle_internal.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_exponential_biased.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_exponential_biased.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_graphcycles_internal.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_graphcycles_internal.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_hash.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_hash.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_int128.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_int128.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_low_level_hash.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_low_level_hash.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_malloc_internal.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_malloc_internal.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_random_internal_platform.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_random_internal_platform.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_random_internal_pool_urbg.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_random_internal_pool_urbg.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_random_internal_randen_hwaes_impl.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_random_internal_randen_hwaes_impl.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_random_internal_randen_hwaes.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_random_internal_randen_hwaes.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_random_internal_randen_slow.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_random_internal_randen_slow.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_random_internal_randen.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_random_internal_randen.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_random_internal_seed_material.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_random_internal_seed_material.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_random_seed_gen_exception.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_random_seed_gen_exception.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_raw_hash_set.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_raw_hash_set.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_raw_logging_internal.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_raw_logging_internal.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_spinlock_wait.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_spinlock_wait.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_stacktrace.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_stacktrace.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_status.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_status.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_statusor.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_statusor.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_str_format_internal.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_str_format_internal.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_strerror.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_strerror.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_strings_internal.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_strings_internal.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_strings.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_strings.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_symbolize.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_symbolize.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_synchronization.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_synchronization.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_throw_delegate.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_throw_delegate.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_time_zone.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_time_zone.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libabsl_time.so.20220623 /usr/lib/x86_64-linux-gnu/libabsl_time.so.20220623
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libaddress_sorting.so.29 /usr/lib/x86_64-linux-gnu/libaddress_sorting.so.29
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libatomic.so.1 /usr/lib/x86_64-linux-gnu/libatomic.so.1
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libboost_context.so.1.81.0 /usr/lib/x86_64-linux-gnu/libboost_context.so.1.81.0
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libboost_filesystem.so.1.81.0 /usr/lib/x86_64-linux-gnu/libboost_filesystem.so.1.81.0
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libboost_log_setup.so.1.81.0 /usr/lib/x86_64-linux-gnu/libboost_log_setup.so.1.81.0
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libboost_log.so.1.81.0 /usr/lib/x86_64-linux-gnu/libboost_log.so.1.81.0
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libboost_thread.so.1.81.0 /usr/lib/x86_64-linux-gnu/libboost_thread.so.1.81.0
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libc.so.6 /usr/lib/x86_64-linux-gnu/libc.so.6
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libcap.so.2 /usr/lib/x86_64-linux-gnu/libcap.so.2
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libcares.so.2 /usr/lib/x86_64-linux-gnu/libcares.so.2
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libcom_err.so.2 /usr/lib/x86_64-linux-gnu/libcom_err.so.2
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libcrypto.so.3 /usr/lib/x86_64-linux-gnu/libcrypto.so.3
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libcrypto++.so.8 /usr/lib/x86_64-linux-gnu/libcrypto++.so.8
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libffi.so.8 /usr/lib/x86_64-linux-gnu/libffi.so.8
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libgcc_s.so.1 /usr/lib/x86_64-linux-gnu/libgcc_s.so.1
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libgcrypt.so.20 /usr/lib/x86_64-linux-gnu/libgcrypt.so.20
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libgmp.so.10 /usr/lib/x86_64-linux-gnu/libgmp.so.10
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libgnutls.so.30 /usr/lib/x86_64-linux-gnu/libgnutls.so.30
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libgpg-error.so.0 /usr/lib/x86_64-linux-gnu/libgpg-error.so.0
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libgpr.so.29 /usr/lib/x86_64-linux-gnu/libgpr.so.29
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libgrpc.so.29 /usr/lib/x86_64-linux-gnu/libgrpc.so.29
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libgrpc++.so.1.51 /usr/lib/x86_64-linux-gnu/libgrpc++.so.1.51
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libgssapi_krb5.so.2 /usr/lib/x86_64-linux-gnu/libgssapi_krb5.so.2
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libhogweed.so.6 /usr/lib/x86_64-linux-gnu/libhogweed.so.6
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libidn2.so.0 /usr/lib/x86_64-linux-gnu/libidn2.so.0
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libjemalloc.so.2 /usr/lib/x86_64-linux-gnu/libjemalloc.so.2
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libk5crypto.so.3 /usr/lib/x86_64-linux-gnu/libk5crypto.so.3
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libkeyutils.so.1 /usr/lib/x86_64-linux-gnu/libkeyutils.so.1
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libkrb5.so.3 /usr/lib/x86_64-linux-gnu/libkrb5.so.3
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libkrb5support.so.0 /usr/lib/x86_64-linux-gnu/libkrb5support.so.0
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/liblber-2.5.so.0 /usr/lib/x86_64-linux-gnu/liblber-2.5.so.0
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libldap-2.5.so.0 /usr/lib/x86_64-linux-gnu/libldap-2.5.so.0
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/liblz4.so.1 /usr/lib/x86_64-linux-gnu/liblz4.so.1
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/liblzf.so.1 /usr/lib/x86_64-linux-gnu/liblzf.so.1
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/liblzma.so.5 /usr/lib/x86_64-linux-gnu/liblzma.so.5
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libm.so.6 /usr/lib/x86_64-linux-gnu/libm.so.6
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libnettle.so.8 /usr/lib/x86_64-linux-gnu/libnettle.so.8
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libp11-kit.so.0 /usr/lib/x86_64-linux-gnu/libp11-kit.so.0
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libpq.so.5 /usr/lib/x86_64-linux-gnu/libpq.so.5
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libprotobuf.so.32 /usr/lib/x86_64-linux-gnu/libprotobuf.so.32
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libre2.so.9 /usr/lib/x86_64-linux-gnu/libre2.so.9
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libresolv.so.2 /usr/lib/x86_64-linux-gnu/libresolv.so.2
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libsasl2.so.2 /usr/lib/x86_64-linux-gnu/libsasl2.so.2
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libssl.so.3 /usr/lib/x86_64-linux-gnu/libssl.so.3
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libstdc++.so.6 /usr/lib/x86_64-linux-gnu/libstdc++.so.6
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libsystemd.so.0 /usr/lib/x86_64-linux-gnu/libsystemd.so.0
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libtasn1.so.6 /usr/lib/x86_64-linux-gnu/libtasn1.so.6
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libunistring.so.2 /usr/lib/x86_64-linux-gnu/libunistring.so.2
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libupb.so.29 /usr/lib/x86_64-linux-gnu/libupb.so.29
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libz.so.1 /usr/lib/x86_64-linux-gnu/libz.so.1
COPY --from=rollups-node /usr/lib/x86_64-linux-gnu/libzstd.so.1 /usr/lib/x86_64-linux-gnu/libzstd.so.1

# STAGE: distroless-arm64
#
# This stage copies the required libraries for arm64 (aarch64).
#
FROM scratch AS distroless-arm64
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_bad_optional_access.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_bad_optional_access.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_bad_variant_access.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_bad_variant_access.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_base.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_base.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_city.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_city.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_cord_internal.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_cord_internal.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_cord.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_cord.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_cordz_functions.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_cordz_functions.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_cordz_handle.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_cordz_handle.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_cordz_info.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_cordz_info.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_debugging_internal.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_debugging_internal.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_demangle_internal.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_demangle_internal.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_exponential_biased.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_exponential_biased.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_graphcycles_internal.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_graphcycles_internal.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_hash.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_hash.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_int128.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_int128.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_low_level_hash.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_low_level_hash.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_malloc_internal.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_malloc_internal.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_random_internal_platform.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_random_internal_platform.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_random_internal_pool_urbg.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_random_internal_pool_urbg.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_random_internal_randen_hwaes_impl.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_random_internal_randen_hwaes_impl.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_random_internal_randen_hwaes.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_random_internal_randen_hwaes.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_random_internal_randen_slow.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_random_internal_randen_slow.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_random_internal_randen.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_random_internal_randen.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_random_internal_seed_material.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_random_internal_seed_material.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_random_seed_gen_exception.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_random_seed_gen_exception.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_raw_hash_set.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_raw_hash_set.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_raw_logging_internal.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_raw_logging_internal.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_spinlock_wait.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_spinlock_wait.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_stacktrace.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_stacktrace.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_status.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_status.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_statusor.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_statusor.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_str_format_internal.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_str_format_internal.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_strerror.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_strerror.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_strings_internal.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_strings_internal.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_strings.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_strings.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_symbolize.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_symbolize.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_synchronization.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_synchronization.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_throw_delegate.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_throw_delegate.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_time_zone.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_time_zone.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libabsl_time.so.20220623 /usr/lib/aarch64-linux-gnu/libabsl_time.so.20220623
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libaddress_sorting.so.29 /usr/lib/aarch64-linux-gnu/libaddress_sorting.so.29
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libatomic.so.1 /usr/lib/aarch64-linux-gnu/libatomic.so.1
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libboost_context.so.1.81.0 /usr/lib/aarch64-linux-gnu/libboost_context.so.1.81.0
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libboost_filesystem.so.1.81.0 /usr/lib/aarch64-linux-gnu/libboost_filesystem.so.1.81.0
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libboost_log_setup.so.1.81.0 /usr/lib/aarch64-linux-gnu/libboost_log_setup.so.1.81.0
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libboost_log.so.1.81.0 /usr/lib/aarch64-linux-gnu/libboost_log.so.1.81.0
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libboost_thread.so.1.81.0 /usr/lib/aarch64-linux-gnu/libboost_thread.so.1.81.0
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libc.so.6 /usr/lib/aarch64-linux-gnu/libc.so.6
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libcap.so.2 /usr/lib/aarch64-linux-gnu/libcap.so.2
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libcares.so.2 /usr/lib/aarch64-linux-gnu/libcares.so.2
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libcom_err.so.2 /usr/lib/aarch64-linux-gnu/libcom_err.so.2
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libcrypto.so.3 /usr/lib/aarch64-linux-gnu/libcrypto.so.3
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libcrypto++.so.8 /usr/lib/aarch64-linux-gnu/libcrypto++.so.8
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libffi.so.8 /usr/lib/aarch64-linux-gnu/libffi.so.8
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libgcc_s.so.1 /usr/lib/aarch64-linux-gnu/libgcc_s.so.1
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libgcrypt.so.20 /usr/lib/aarch64-linux-gnu/libgcrypt.so.20
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libgmp.so.10 /usr/lib/aarch64-linux-gnu/libgmp.so.10
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libgnutls.so.30 /usr/lib/aarch64-linux-gnu/libgnutls.so.30
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libgpg-error.so.0 /usr/lib/aarch64-linux-gnu/libgpg-error.so.0
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libgpr.so.29 /usr/lib/aarch64-linux-gnu/libgpr.so.29
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libgrpc.so.29 /usr/lib/aarch64-linux-gnu/libgrpc.so.29
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libgrpc++.so.1.51 /usr/lib/aarch64-linux-gnu/libgrpc++.so.1.51
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libgssapi_krb5.so.2 /usr/lib/aarch64-linux-gnu/libgssapi_krb5.so.2
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libhogweed.so.6 /usr/lib/aarch64-linux-gnu/libhogweed.so.6
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libidn2.so.0 /usr/lib/aarch64-linux-gnu/libidn2.so.0
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libjemalloc.so.2 /usr/lib/aarch64-linux-gnu/libjemalloc.so.2
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libk5crypto.so.3 /usr/lib/aarch64-linux-gnu/libk5crypto.so.3
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libkeyutils.so.1 /usr/lib/aarch64-linux-gnu/libkeyutils.so.1
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libkrb5.so.3 /usr/lib/aarch64-linux-gnu/libkrb5.so.3
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libkrb5support.so.0 /usr/lib/aarch64-linux-gnu/libkrb5support.so.0
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/liblber-2.5.so.0 /usr/lib/aarch64-linux-gnu/liblber-2.5.so.0
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libldap-2.5.so.0 /usr/lib/aarch64-linux-gnu/libldap-2.5.so.0
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/liblz4.so.1 /usr/lib/aarch64-linux-gnu/liblz4.so.1
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/liblzf.so.1 /usr/lib/aarch64-linux-gnu/liblzf.so.1
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/liblzma.so.5 /usr/lib/aarch64-linux-gnu/liblzma.so.5
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libm.so.6 /usr/lib/aarch64-linux-gnu/libm.so.6
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libnettle.so.8 /usr/lib/aarch64-linux-gnu/libnettle.so.8
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libp11-kit.so.0 /usr/lib/aarch64-linux-gnu/libp11-kit.so.0
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libpq.so.5 /usr/lib/aarch64-linux-gnu/libpq.so.5
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libprotobuf.so.32 /usr/lib/aarch64-linux-gnu/libprotobuf.so.32
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libre2.so.9 /usr/lib/aarch64-linux-gnu/libre2.so.9
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libresolv.so.2 /usr/lib/aarch64-linux-gnu/libresolv.so.2
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libsasl2.so.2 /usr/lib/aarch64-linux-gnu/libsasl2.so.2
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libssl.so.3 /usr/lib/aarch64-linux-gnu/libssl.so.3
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libstdc++.so.6 /usr/lib/aarch64-linux-gnu/libstdc++.so.6
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libsystemd.so.0 /usr/lib/aarch64-linux-gnu/libsystemd.so.0
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libtasn1.so.6 /usr/lib/aarch64-linux-gnu/libtasn1.so.6
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libunistring.so.2 /usr/lib/aarch64-linux-gnu/libunistring.so.2
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libupb.so.29 /usr/lib/aarch64-linux-gnu/libupb.so.29
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libz.so.1 /usr/lib/aarch64-linux-gnu/libz.so.1
COPY --from=rollups-node /usr/lib/aarch64-linux-gnu/libzstd.so.1 /usr/lib/aarch64-linux-gnu/libzstd.so.1

# STAGE: distroless-dependencies
#
# This stages acts as intermediary to copy the dependencies for the TARGETARCH architecture.
#
FROM distroless-${TARGETARCH} as distroless-dependencies

# STAGE: rollups-node-distroless
#
# This is the final stage for the rollups-node:${TAG}-distroless target.
#
FROM gcr.io/distroless/base-debian12:nonroot as rollups-node-distroless

# copy $TARGETARCH dependencies
COPY --from=distroless-dependencies / /

# Copy server-manager
COPY --from=server-manager /usr/bin/server-manager /usr/bin

# Copy cartesi-rollups-* Rust binaries.
ARG RUST_BUILD_PATH
ARG RUST_TARGET=${RUST_BUILD_PATH}/target/release
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-authority-claimer /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-advance-runner /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-authority-claimer /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-dispatcher /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-graphql-server /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-host-runner /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-indexer /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-inspect-server /usr/bin
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-state-server /usr/bin

# Copy server-manager and machine-emulator binaries and dependencies
COPY --from=rollups-node /usr/bin/cartesi-machine /usr/bin/cartesi-machine
COPY --from=rollups-node /usr/bin/cartesi-machine-stored-hash /usr/bin/cartesi-machine-stored-hash
COPY --from=rollups-node /usr/bin/jsonrpc-remote-cartesi-machine /usr/bin/jsonrpc-remote-cartesi-machine
COPY --from=rollups-node /usr/bin/merkle-tree-hash /usr/bin/merkle-tree-hash
COPY --from=rollups-node /usr/bin/remote-cartesi-machine /usr/bin/remote-cartesi-machine
COPY --from=rollups-node /usr/bin/rollup-memory-range /usr/bin/rollup-memory-range
COPY --from=rollups-node /usr/lib/libcartesi-0.16.so /usr/lib/libcartesi-0.16.so
COPY --from=rollups-node /usr/lib/libcartesi_grpc-0.16.so /usr/lib/libcartesi_grpc-0.16.so
COPY --from=rollups-node /usr/lib/libcartesi_jsonrpc-0.16.so /usr/lib/libcartesi_jsonrpc-0.16.so

# Copy Go binary.
ARG GO_BUILD_PATH
COPY --from=go-builder ${GO_BUILD_PATH}/cartesi-rollups-node /usr/bin

# Copy redis-server
COPY --from=rollups-node /usr/bin/redis-server /usr/bin/redis-server

CMD [ "/usr/bin/cartesi-rollups-node" ]
