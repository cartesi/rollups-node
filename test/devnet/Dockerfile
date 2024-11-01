ARG EMULATOR_VERSION=0.18.1
ARG FOUNDRY_NIGHTLY_VERSION=2044faec64f99a21f0e5f0094458a973612d0712
ARG CONTRACTS_VERSION=2.0.0-rc.12
ARG DEVNET_BUILD_PATH=/opt/cartesi/rollups-contracts

FROM cartesi/machine-emulator:${EMULATOR_VERSION} AS rollups-node-devnet
ARG FOUNDRY_NIGHTLY_VERSION
ARG CONTRACTS_VERSION
ARG DEVNET_BUILD_PATH

USER root

# Install ca-certificates, curl, and git (setup).
ENV DEBIAN_FRONTEND=noninteractive
RUN <<EOF
    set -e
    apt-get update
    apt-get install -y --no-install-recommends ca-certificates curl git jq
EOF

# Install Foundry from downloaded pre-compiled binaries.
ARG FOUNDRY_NIGHTLY_VERSION
RUN <<EOF
    set -e
    URL=https://github.com/foundry-rs/foundry/releases/download
    VERSION=nightly-${FOUNDRY_NIGHTLY_VERSION}
    ARCH=$(dpkg --print-architecture)
    ARTIFACT=foundry_nightly_linux_${ARCH}.tar.gz
    curl -sSL ${URL}/${VERSION}/${ARTIFACT} -o /tmp/foundry.tar.gz
    case $ARCH in
    amd64) echo "c53d77f7f0266ef8bc9daf8d4a8ce39ca5fdce859b5d028b1e55bded7da687e2  /tmp/foundry.tar.gz" | shasum -a 256 --check ;;
    arm64) echo "5fe5913841c56b79ee5c15aefc2e9730bd6beb07cba0e2112447b822d78b079d  /tmp/foundry.tar.gz" | shasum -a 256 --check ;;
    esac
    tar -zxf /tmp/foundry.tar.gz -C /usr/local/bin
EOF

# Install anvil state file and devnet deployment info.
RUN mkdir -p ${DEVNET_BUILD_PATH}
RUN cat <<'EOF' > ${DEVNET_BUILD_PATH}/deployment.jq
    . as $root | .contracts |
    reduce to_entries[] as $c (
        {"name": $root.name, "ChainId": $root.chainId | tonumber};
        . + {($c.key): ($c.value.address)}
    )
EOF

RUN <<EOF
    set -e
    URL=https://github.com/cartesi/rollups-contracts/releases/download
    VERSION=v${CONTRACTS_VERSION}
    ARTIFACT=rollups-contracts-${CONTRACTS_VERSION}-anvil-nightly-${FOUNDRY_NIGHTLY_VERSION}.tar.gz
    curl -sSL ${URL}/${VERSION}/${ARTIFACT} -o /tmp/contracts.tar.gz
    echo "691d229e16b866f97ee5cf27b88573d8434f064a959061c8755e91a048c2a811  /tmp/contracts.tar.gz" | sha256sum --check
    tar -zxf /tmp/contracts.tar.gz -C ${DEVNET_BUILD_PATH}/
    mkdir -p /usr/share/devnet
    jq -cf ${DEVNET_BUILD_PATH}/deployment.jq < ${DEVNET_BUILD_PATH}/localhost.json > /usr/share/devnet/deployment.json
    mv ${DEVNET_BUILD_PATH}/state.json /usr/share/devnet/anvil_state.json
EOF

HEALTHCHECK --interval=1s --timeout=1s --retries=5 \
	CMD curl \
	-X \
	POST \
	-s \
	-H 'Content-Type: application/json' \
	-d '{"jsonrpc":"2.0","id":"1","method":"net_listening","params":[]}' \
	http://127.0.0.1:8545

CMD ["anvil", "--block-time", "1", "--host", "0.0.0.0", "--load-state", "/usr/share/devnet/anvil_state.json"]
