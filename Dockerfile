# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

# syntax=docker.io/docker/dockerfile:1

ARG EMULATOR_VERSION=0.18.1
ARG RUST_VERSION=1.78.0

# Build directories.
ARG RUST_BUILD_PATH=/build/cartesi/rust
ARG GO_BUILD_PATH=/build/cartesi/go

FROM cartesi/machine-emulator:${EMULATOR_VERSION} AS common-env

USER root

# Re-declare ARGs so they can be used in the RUN block
ARG RUST_BUILD_PATH
ARG GO_BUILD_PATH

# Install ca-certificates and curl (setup).
RUN <<EOF
    set -e
    apt-get update
    apt-get install -y --no-install-recommends ca-certificates curl wget build-essential pkg-config libssl-dev
    mkdir -p /opt/rust/rustup /opt/go ${RUST_BUILD_PATH} ${GO_BUILD_PATH}/rollups-node
    chown -R cartesi:cartesi /opt/rust /opt/go ${RUST_BUILD_PATH} ${GO_BUILD_PATH}
EOF

USER cartesi

# =============================================================================
# STAGE: rust-installer
#
# - Install rust and cargo-chef.
# =============================================================================

FROM common-env AS rust-installer

# Get Rust
ENV CARGO_HOME=/opt/rust/cargo
ENV RUSTUP_HOME=/opt/rust/rustup

RUN <<EOF
    set -e
    cd /tmp
    wget https://github.com/rust-lang/rustup/archive/refs/tags/1.27.0.tar.gz
    echo "3d331ab97d75b03a1cc2b36b2f26cd0a16d681b79677512603f2262991950ad1  1.27.0.tar.gz" | sha256sum --check
    tar xzf 1.27.0.tar.gz
    bash rustup-1.27.0/rustup-init.sh \
        -y \
        --no-modify-path \
        --default-toolchain 1.78 \
        --component rustfmt \
        --profile minimal
    rm -rf 1.27.0*
    $CARGO_HOME/bin/cargo install cargo-chef
EOF

ENV PATH="${CARGO_HOME}/bin:${PATH}"

ARG RUST_BUILD_PATH
WORKDIR ${RUST_BUILD_PATH}

# =============================================================================
# STAGE: rust-prepare
#
# This stage prepares the recipe with just the external dependencies.
# =============================================================================

FROM rust-installer AS rust-prepare
COPY ./cmd/authority-claimer/ .
RUN cargo chef prepare --recipe-path recipe.json

# =============================================================================
# STAGE: rust-builder
#
# This stage builds the Rust binaries. First it builds the external
# dependencies and then it builds the node binaries.
# =============================================================================

FROM rust-installer AS rust-builder

# Build external dependencies with cargo chef.
COPY --from=rust-prepare ${RUST_BUILD_PATH}/recipe.json .
RUN cargo chef cook --release --recipe-path recipe.json

# Build application.
COPY ./cmd/authority-claimer/ .
RUN cargo build --release

# =============================================================================
# STAGE: go-installer
#
# This stage installs Go in the /opt directory.
# =============================================================================

FROM common-env AS go-installer
# Download and verify Go based on the target architecture
RUN <<EOF
    set -e
    ARCH=$(dpkg --print-architecture)
    wget -O /tmp/go.tar.gz "https://go.dev/dl/go1.22.7.linux-${ARCH}.tar.gz"
    sha256sum /tmp/go.tar.gz
    case "$ARCH" in
        amd64) echo "fc5d49b7a5035f1f1b265c17aa86e9819e6dc9af8260ad61430ee7fbe27881bb  /tmp/go.tar.gz" | sha256sum --check ;;
        arm64) echo "ed695684438facbd7e0f286c30b7bc2411cfc605516d8127dc25c62fe5b03885  /tmp/go.tar.gz" | sha256sum --check ;;
        *) echo "unsupported architecture: $ARCH"; exit 1 ;;
    esac
    tar -C /opt -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz
EOF

# Set up Go environment variables
ENV PATH="/opt/go/bin:$PATH"

# =============================================================================
# STAGE: go-prepare
#
# This stage prepares the Go build environment. It downloads the external
# =============================================================================

FROM go-installer AS go-prepare

ARG GO_BUILD_PATH
WORKDIR ${GO_BUILD_PATH}

ENV GOCACHE=${GO_BUILD_PATH}/.cache
ENV GOENV=${GO_BUILD_PATH}/.config/go/env
ENV GOPATH=${GO_BUILD_PATH}/.go

# Download external dependencies.
COPY go.mod ${GO_BUILD_PATH}/rollups-node/
COPY go.sum ${GO_BUILD_PATH}/rollups-node/
RUN cd ${GO_BUILD_PATH}/rollups-node && go mod download

# =============================================================================
# STAGE: go-builder
#
# This stage builds the node Go binaries. First it downloads the external
# dependencies and then it builds the binaries.
# =============================================================================

FROM go-prepare AS go-builder

ARG GO_BUILD_PATH
# Build application.
COPY --chown=cartesi:cartesi . ${GO_BUILD_PATH}/rollups-node/
RUN cd ${GO_BUILD_PATH}/rollups-node && make build-go

# =============================================================================
# STAGE: rollups-node
#
# This stage prepares the final Docker image that will be used in the production
# environment. It installs in /usr/bin all the binaries necessary to run the
# node.
#
# (This stage copies the binaries from previous stages.)
# =============================================================================

FROM cartesi/machine-emulator:${EMULATOR_VERSION} AS rollups-node

ARG NODE_RUNTIME_DIR=/var/lib/cartesi-rollups-node

USER root

# Download system dependencies required at runtime.
ARG DEBIAN_FRONTEND=noninteractive
RUN <<EOF
    set -e
    apt-get update
    apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        procps
    rm -rf /var/lib/apt/lists/*
    mkdir -p ${NODE_RUNTIME_DIR}/snapshots ${NODE_RUNTIME_DIR}/data
    chown -R cartesi:cartesi ${NODE_RUNTIME_DIR}
EOF

# Copy Rust binaries.
# Explicitly copy each binary to avoid adding unnecessary files to the runtime
# image.
ARG RUST_BUILD_PATH
ARG RUST_TARGET=${RUST_BUILD_PATH}/target/release
COPY --from=rust-builder ${RUST_TARGET}/cartesi-rollups-authority-claimer /usr/bin

# Copy Go binary.
ARG GO_BUILD_PATH
COPY --from=go-builder ${GO_BUILD_PATH}/rollups-node/cartesi-rollups-* /usr/bin

# Set user to low-privilege.
USER cartesi

WORKDIR ${NODE_RUNTIME_DIR}

HEALTHCHECK --interval=1s --timeout=1s --retries=5 \
    CMD curl -G -f -H 'Content-Type: application/json' http://127.0.0.1:10001/healthz

# Set the Go supervisor as the command.
CMD [ "cartesi-rollups-node" ]
