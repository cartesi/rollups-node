ARG FOUNDRY_VERSION=1.5.1
ARG PRT_CONTRACTS_VERSION=3.0.0-alpha.4
ARG DEVNET_BUILD_PATH=/opt/cartesi/rollups-contracts

FROM debian:trixie-20250811 AS rollups-node-devnet
ARG FOUNDRY_VERSION
ARG PRT_CONTRACTS_VERSION
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
RUN <<EOF
    set -e
    URL=https://github.com/foundry-rs/foundry/releases/download
    ARCH=$(dpkg --print-architecture)
    ARTIFACT=foundry_v${FOUNDRY_VERSION}_linux_${ARCH}.tar.gz
    curl -sSL ${URL}/v${FOUNDRY_VERSION}/${ARTIFACT} -o /tmp/foundry.tar.gz
    case $ARCH in
    amd64) echo "73640b01bd9ed29fdb4965085099371f8cf0dbbec3e2086cf54564efc4dcfe88  /tmp/foundry.tar.gz" | shasum -a 256 --check ;;
    arm64) echo "cccf28bdf202289e837a9e21ed213b2b80dc1e806e12f1717bc98a44315c331e  /tmp/foundry.tar.gz" | shasum -a 256 --check ;;
    esac
    tar -zxf /tmp/foundry.tar.gz -C /usr/local/bin
EOF

# Install Anvil state and devnet deployment information.
# The release includes TestUsdWithdrawalOutputBuilder backed by TestUsdc.
RUN <<EOF
    set -e
    mkdir -p ${DEVNET_BUILD_PATH}
    URL=https://github.com/cartesi/dave/releases/download
    VERSION=v${PRT_CONTRACTS_VERSION}
    ARTIFACT=cartesi-rollups-prt-${PRT_CONTRACTS_VERSION}-anvil-${FOUNDRY_VERSION}.tar.gz
    curl -sSL ${URL}/${VERSION}/${ARTIFACT} -o /tmp/contracts.tar.gz
    echo "ed10a8077113c426a298bb3592ca3725c31aad0828c7e7853b655593db2cd006  /tmp/contracts.tar.gz" | sha256sum --check
    tar -zxf /tmp/contracts.tar.gz -C ${DEVNET_BUILD_PATH}/
    mkdir -p /usr/share/devnet

    cat ${DEVNET_BUILD_PATH}/deployments/31337/*.json | jq -s 'map({ (.contractName): .address }) | add' > /usr/share/devnet/deployment.json
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

CMD ["anvil", "--block-time", "1", "--mixed-mining", "--silent", "--host", "0.0.0.0", "--chain-id", "31337", "--preserve-historical-states", "--max-persisted-states", "86400", "--load-state", "/usr/share/devnet/anvil_state.json"]
