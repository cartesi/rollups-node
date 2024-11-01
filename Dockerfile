# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

# syntax=docker.io/docker/dockerfile:1

ARG EMULATOR_VERSION=0.18.1

# Build directories.
ARG GO_BUILD_PATH=/build/cartesi/go

FROM cartesi/machine-emulator:${EMULATOR_VERSION} AS common-env

USER root

# Re-declare ARGs so they can be used in the RUN block
ARG GO_BUILD_PATH

# Install ca-certificates and curl (setup).
RUN <<EOF
    set -e
    apt-get update
    apt-get install -y --no-install-recommends ca-certificates curl wget build-essential pkg-config libssl-dev
    mkdir -p /opt/go ${GO_BUILD_PATH}/rollups-node
    chown -R cartesi:cartesi /opt/go ${GO_BUILD_PATH}
EOF

USER cartesi

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
COPY --chown=cartesi:cartesi go.mod ${GO_BUILD_PATH}/rollups-node/
COPY --chown=cartesi:cartesi go.sum ${GO_BUILD_PATH}/rollups-node/
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

# Copy Go binary.
ARG GO_BUILD_PATH
COPY --from=go-builder ${GO_BUILD_PATH}/rollups-node/cartesi-rollups-* /usr/bin

# Set user to low-privilege.
USER cartesi

WORKDIR ${NODE_RUNTIME_DIR}

HEALTHCHECK --interval=1s --timeout=1s --retries=5 \
    CMD curl -G -f -H 'Content-Type: application/json' http://127.0.0.1:10000/healthz

# Set the Go supervisor as the command.
CMD [ "cartesi-rollups-node" ]
