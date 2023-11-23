# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

FROM cartesi/server-manager:0.8.2 as build-machine-stage

USER root

# Install system dependencies
RUN apt update && \
    apt install -y wget

# Download rootfs, linux and rom
ENV IMAGES_PATH /usr/share/cartesi-machine/images
RUN wget -O ${IMAGES_PATH}/rootfs.ext2 https://github.com/cartesi/image-rootfs/releases/download/v0.18.0/rootfs-v0.18.0.ext2 && \
    wget -O ${IMAGES_PATH}/linux.bin https://github.com/cartesi/image-kernel/releases/download/v0.17.0/linux-5.15.63-ctsi-2-v0.17.0.bin && \
    wget -O ${IMAGES_PATH}/rom.bin https://github.com/cartesi/machine-emulator-rom/releases/download/v0.17.0/rom-v0.17.0.bin

# Generate machine with echo and store it
ENV SNAPSHOT_DIR=/tmp/dapp-bin
RUN cartesi-machine \
    --ram-length=128Mi \
    --rollup \
    --store=$SNAPSHOT_DIR \
    -- "ioctl-echo-loop --vouchers=1 --notices=1 --reports=1 --verbose=1"

FROM debian:bookworm-20230725-slim as machine-stage

RUN addgroup --system --gid 102 cartesi && \
    adduser --system --uid 102 --ingroup cartesi --disabled-login --no-create-home --home /nonexistent --gecos "cartesi user" --shell /bin/false cartesi

COPY --from=build-machine-stage --chown=cartesi:cartesi /tmp/dapp-bin /var/opt/cartesi/machine-snapshots/0_0
RUN ln -s /var/opt/cartesi/machine-snapshots/0_0 /var/opt/cartesi/machine-snapshots/latest

WORKDIR /var/opt/cartesi/machine-snapshots
ENTRYPOINT [ "/bin/bash" ]
