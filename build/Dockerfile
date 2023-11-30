# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

# syntax=docker.io/docker/dockerfile:1.4

ARG RUST_VERSION=1.74.0
ARG GO_VERSION=1.21.1
ARG SERVER_MANAGER_VERSION=0.8.2
ARG ROLLUPS_CONTRACTS_VERSION=1.1.0

ARG BASE_PATH=/opt/cartesi
ARG RUST_BUILD_PATH=${BASE_PATH}/src/rollups-node/offchain
ARG GO_BASE_PATH=/go
ARG GO_BUILD_PATH=${GO_BASE_PATH}/cartesi-rollups-node
ARG GO_BIN_PATH=${GO_BASE_PATH}/bin
ARG RUNTIME_DIR=/var/opt/cartesi

#
# Cargo chef
#
FROM rust:${RUST_VERSION}-bookworm AS rust-chef

RUN rustup component add rustfmt
RUN cargo install cargo-chef

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
RUN apt-get update && apt-get install -y --no-install-recommends \
        cmake libprotobuf-dev protobuf-compiler curl

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
RUN apt-get update && apt-get install -y --no-install-recommends \
    libpq5 ca-certificates curl
RUN rm -rf /var/lib/apt/lists/*

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
RUN mkdir -p ${RUNTIME_DIR}
RUN chown cartesi:cartesi ${RUNTIME_DIR}
WORKDIR ${RUNTIME_DIR}

USER cartesi
CMD ["/bin/bash"]
