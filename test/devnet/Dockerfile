ARG FOUNDRY_VERSION=1.4.3
ARG PRT_CONTRACTS_VERSION=2.1.0
ARG DEVNET_BUILD_PATH=/opt/cartesi/rollups-contracts

FROM debian:bookworm-20250407 AS rollups-node-devnet
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
    amd64) echo "325ba04dc5cb41c110723b00ac291f8269f8cd785028299aad8252ef980961a7  /tmp/foundry.tar.gz" | shasum -a 256 --check ;;
    arm64) echo "209492cb4ebd723d9eac002fa30f41f53c8810105b67d3c32fe8201cf70f89d4  /tmp/foundry.tar.gz" | shasum -a 256 --check ;;
    esac
    tar -zxf /tmp/foundry.tar.gz -C /usr/local/bin
EOF

# Install anvil state file and devnet deployment info.
RUN <<EOF
    set -e
    mkdir -p ${DEVNET_BUILD_PATH}
    URL=https://github.com/cartesi/dave/releases/download
    VERSION=v${PRT_CONTRACTS_VERSION}
    ARTIFACT=cartesi-rollups-prt-${PRT_CONTRACTS_VERSION}-anvil-v${FOUNDRY_VERSION}.tar.gz
    curl -sSL ${URL}/${VERSION}/${ARTIFACT} -o /tmp/contracts.tar.gz
    echo "049179a3e4443fa4b13847fe155c4fd4fd781c7a5cabcef6d2a5abe4d401f7c8  /tmp/contracts.tar.gz" | sha256sum --check
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

CMD ["anvil", "--block-time", "1", "--silent", "--host", "0.0.0.0", "--chain-id", "31337", "--max-persisted-states", "86400", "--load-state", "/usr/share/devnet/anvil_state.json"]
