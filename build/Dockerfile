# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

# syntax=docker.io/docker/dockerfile:1.4

ARG RUST_VERSION=1.72.1
ARG SERVER_MANAGER_VERSION=0.8.2
ARG ROLLUPS_CONTRACTS_VERSION=1.0.2

ARG BASE_PATH=/opt/cartesi
ARG RUST_BUILD_PATH=${BASE_PATH}/src/rollups-node/offchain
ARG DEPLOYMENT_PATH=${BASE_PATH}/share/deployments
ARG RUNTIME_DIR=/var/opt/cartesi

#
# On-chain deployment files
#
FROM debian:bookworm-slim as deployment-download

# Download deployment files from npm
WORKDIR /tmp/deployment
ARG ROLLUPS_CONTRACTS_VERSION
ADD https://registry.npmjs.org/@cartesi/rollups/-/rollups-${ROLLUPS_CONTRACTS_VERSION}.tgz .
COPY ./build/shasumfile .
RUN sha256sum -c shasumfile
RUN tar zxf rollups-${ROLLUPS_CONTRACTS_VERSION}.tgz

FROM scratch as deployment
ARG DEPLOYMENT_PATH
WORKDIR ${DEPLOYMENT_PATH}
COPY --from=deployment-download /tmp/deployment/package/export/abi/* .

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
# Runtime
#
FROM cartesi/server-manager:${SERVER_MANAGER_VERSION} as rollups-node

USER root

ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    libpq5 ca-certificates curl
RUN rm -rf /var/lib/apt/lists/*

# Copy deployment files
ARG DEPLOYMENT_PATH
WORKDIR ${DEPLOYMENT_PATH}
COPY --from=deployment ${DEPLOYMENT_PATH}/*.json .

# Copy Rust binaries
ARG BASE_PATH
WORKDIR ${BASE_PATH}/bin
ARG RUST_BUILD_PATH
COPY --from=rust-builder ${RUST_BUILD_PATH}/target/release/cartesi-rollups-* .
ENV PATH="${BASE_PATH}/bin:${PATH}"

# Setup runtime dir
ARG RUNTIME_DIR
RUN mkdir -p ${RUNTIME_DIR}
RUN chown cartesi:cartesi ${RUNTIME_DIR}
WORKDIR ${RUNTIME_DIR}

USER cartesi
CMD ["/bin/bash"]
