# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

# syntax=docker.io/docker/dockerfile:1.4

# deps install
FROM rust:1.71.0-bookworm AS chef

ENV CARGO_REGISTRIES_CARTESI_INDEX=https://github.com/cartesi/crates-index
RUN rustup component add rustfmt
RUN cargo install cargo-chef

# cargo chef prepare stage
FROM chef AS planner

WORKDIR /usr/src/app/offchain
COPY . /usr/src/app
RUN cargo chef prepare --recipe-path recipe.json

# cargo chef cook stage
FROM chef AS builder

RUN <<EOF
DEBIAN_FRONTEND="noninteractive" apt-get update
DEBIAN_FRONTEND="noninteractive" apt-get install -y --no-install-recommends cmake libprotobuf-dev protobuf-compiler curl
EOF

WORKDIR /usr/src/app/offchain
COPY --from=planner /usr/src/app/offchain/recipe.json recipe.json
RUN cargo chef cook --release --recipe-path recipe.json

WORKDIR /usr/src/app/offchain
COPY . /usr/src/app

RUN cargo build --release

RUN mkdir -p /opt/cartesi/share/deployments
RUN curl "https://registry.npmjs.org/@cartesi/rollups/-/rollups-1.0.1.tgz" -o /tmp/rollups.tgz && \
    tar zxf /tmp/rollups.tgz -C /tmp && \
    cp /tmp/package/export/abi/* /opt/cartesi/share/deployments

# define runtime image
FROM debian:bookworm-20230814-slim as runtime
RUN addgroup --system --gid 102 cartesi && \
    adduser --system --uid 102 --ingroup cartesi --disabled-login --no-create-home --home /nonexistent --gecos "cartesi user" --shell /bin/false cartesi
RUN mkdir -p /var/opt/cartesi
RUN chown cartesi:cartesi /var/opt/cartesi
WORKDIR /var/opt/cartesi
ENV PATH="/opt/cartesi/bin:${PATH}"

## runtimes
FROM runtime AS state_server
RUN <<EOF
apt-get update
DEBIAN_FRONTEND="noninteractive" apt-get install -y --no-install-recommends ca-certificates
rm -rf /var/lib/apt/lists/*
EOF
COPY --from=builder /usr/src/app/offchain/target/release/cartesi-rollups-state-server /opt/cartesi/bin/cartesi-rollups-state-server
USER cartesi
ENTRYPOINT ["/opt/cartesi/bin/cartesi-rollups-state-server"]

FROM runtime AS indexer
RUN <<EOF
apt-get update
DEBIAN_FRONTEND="noninteractive" apt-get install -y --no-install-recommends libpq5 ca-certificates
rm -rf /var/lib/apt/lists/*
EOF
COPY --from=builder /usr/src/app/offchain/target/release/cartesi-rollups-indexer /opt/cartesi/bin/cartesi-rollups-indexer
USER cartesi
ENTRYPOINT ["/opt/cartesi/bin/cartesi-rollups-indexer"]

FROM runtime AS dispatcher
RUN <<EOF
apt-get update
DEBIAN_FRONTEND="noninteractive" apt-get install -y --no-install-recommends ca-certificates
rm -rf /var/lib/apt/lists/*
EOF
COPY --from=builder /usr/src/app/offchain/target/release/cartesi-rollups-dispatcher /opt/cartesi/bin/cartesi-rollups-dispatcher
COPY --from=builder /opt/cartesi/share/deployments /opt/cartesi/share/deployments
USER cartesi
ENTRYPOINT ["/opt/cartesi/bin/cartesi-rollups-dispatcher"]

FROM runtime AS advance_runner
RUN <<EOF
apt-get update
DEBIAN_FRONTEND="noninteractive" apt-get install -y --no-install-recommends ca-certificates curl
rm -rf /var/lib/apt/lists/*
EOF
COPY --from=builder /usr/src/app/offchain/target/release/cartesi-rollups-advance-runner /opt/cartesi/bin/cartesi-rollups-advance-runner
USER cartesi
ENTRYPOINT ["/opt/cartesi/bin/cartesi-rollups-advance-runner"]

FROM runtime AS inspect_server
COPY --from=builder /usr/src/app/offchain/target/release/cartesi-rollups-inspect-server /opt/cartesi/bin/cartesi-rollups-inspect-server
USER cartesi
ENTRYPOINT ["/opt/cartesi/bin/cartesi-rollups-inspect-server"]

FROM runtime AS graphql_server
RUN <<EOF
apt-get update
DEBIAN_FRONTEND="noninteractive" apt-get install -y --no-install-recommends libpq5
rm -rf /var/lib/apt/lists/*
EOF
COPY --from=builder /usr/src/app/offchain/target/release/cartesi-rollups-graphql-server /opt/cartesi/bin/cartesi-rollups-graphql-server
USER cartesi
ENTRYPOINT ["/opt/cartesi/bin/cartesi-rollups-graphql-server"]

FROM runtime AS host_runner
COPY --from=builder /usr/src/app/offchain/target/release/cartesi-rollups-host-runner /opt/cartesi/bin/cartesi-rollups-host-runner
USER cartesi
ENTRYPOINT ["/opt/cartesi/bin/cartesi-rollups-host-runner"]
