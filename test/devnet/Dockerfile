ARG FOUNDRY_VERSION=1.1.0
ARG CONTRACTS_VERSION=1.0.1-test
ARG DEVNET_BUILD_PATH=/opt/cartesi/rollups-contracts

FROM debian:bookworm-20250407 AS rollups-node-devnet
ARG FOUNDRY_VERSION
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
RUN <<EOF
    set -e
    URL=https://github.com/foundry-rs/foundry/releases/download
    ARCH=$(dpkg --print-architecture)
    ARTIFACT=foundry_v${FOUNDRY_VERSION}_linux_${ARCH}.tar.gz
    curl -sSL ${URL}/v${FOUNDRY_VERSION}/${ARTIFACT} -o /tmp/foundry.tar.gz
    case $ARCH in
    amd64) echo "22112a68962e0d1238aad3a33d888f399dfd9b6d9e981d801ffd09cb5357a9a4  /tmp/foundry.tar.gz" | shasum -a 256 --check ;;
    arm64) echo "ddcde0cd99cc71080d86f44a5d193d4dad089b33902e7278ae0a9dc7ad380d42  /tmp/foundry.tar.gz" | shasum -a 256 --check ;;
    esac
    tar -zxf /tmp/foundry.tar.gz -C /usr/local/bin
EOF

# Install anvil state file and devnet deployment info.
RUN <<EOF
    set -e
    mkdir -p ${DEVNET_BUILD_PATH}
    URL=https://github.com/cartesi/dave/releases/download
    VERSION=v${CONTRACTS_VERSION}
    ARTIFACT=cartesi-rollups-prt-anvil-v${FOUNDRY_VERSION}.tar.gz
    curl -sSL ${URL}/${VERSION}/${ARTIFACT} -o /tmp/contracts.tar.gz
    echo "88e6d398862ef1669aca827fb2163a85e22e5edfb15b51e69d2107ec5905bc3c  /tmp/contracts.tar.gz" | sha256sum --check
    tar -zxf /tmp/contracts.tar.gz -C ${DEVNET_BUILD_PATH}/
    mkdir -p /usr/share/devnet
    cat ${DEVNET_BUILD_PATH}/deployments/*.json | jq -s 'map({ (.contractName): .address }) | add' > /usr/share/devnet/deployment.json
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

CMD ["anvil", "--block-time", "1", "--silent", "--host", "0.0.0.0", "--chain-id", "13370", "--load-state", "/usr/share/devnet/anvil_state.json"]
