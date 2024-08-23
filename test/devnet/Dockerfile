ARG EMULATOR_VERSION=0.18.1
ARG FOUNDRY_NIGHTLY_VERSION=9dbfb2f1115466b28f2697e158131f90df6b2590

FROM  cartesi/machine-emulator:${EMULATOR_VERSION} AS devnet-base

USER root

# Install ca-certificates, curl, and git (setup).
ENV DEBIAN_FRONTEND=noninteractive
RUN <<EOF
    set -e
    apt-get update
    apt-get install -y --no-install-recommends ca-certificates curl git
EOF

# Install Foundry from downloaded pre-compiled binaries.
ARG FOUNDRY_NIGHTLY_VERSION
RUN <<EOF
    set -ex
    URL=https://github.com/foundry-rs/foundry/releases/download
    VERSION=nightly-${FOUNDRY_NIGHTLY_VERSION}
    ARCH=$(dpkg --print-architecture)
    ARTIFACT=foundry_nightly_linux_${ARCH}.tar.gz
    curl -sSL ${URL}/${VERSION}/${ARTIFACT} | tar -zx -C /usr/local/bin
EOF

# =============================================================================
# STAGE: devnet-deployer
#
# This stage builds the devnet state that will be loaded in Anvil.
# =============================================================================

FROM devnet-base AS devnet-deployer

# Install nodejs & pnpm.
ENV DEBIAN_FRONTEND=noninteractive
RUN <<EOF
    set -e
    apt-get install -y --no-install-recommends gnupg jq
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
    apt-get install -y --no-install-recommends nodejs
    npm install -g @pnpm/exe
EOF

COPY rollups-contracts /opt/cartesi/rollups-contracts

RUN <<EOF
    set -e
    cd /opt/cartesi/rollups-contracts
    anvil --preserve-historical-states --dump-state=anvil_state.json > anvil.log 2>&1 &
    ANVIL_PID=$!
    pnpm install
    if ! pnpm run deploy:development; then
        echo "Failed to deploy contracts"
        cat anvil.log
        exit 1
    fi
    cd deployments/localhost
    jq -c -n --argjson chainId $(cat .chainId) '[inputs | { (input_filename | gsub(".*/|[.]json$"; "")) : .address }] | add | .ChainId = $chainId' *.json > ../../deployment.json
    kill -15 ${ANVIL_PID}
    elapsed=0
    TIMEOUT=30
    while kill -0 ${ANVIL_PID} 2>/dev/null; do
	if [ $elapsed -ge $TIMEOUT ]; then
	    echo "Error:  Anvil state dump timed out after ${TIMEOUT} seconds" >&2
	    exit 1
	fi
	sleep 1
	elapsed=$((elapsed + 1))
    done
EOF

# =============================================================================
# STAGE: rollups-node-devnet
#
# This stage contains the Ethereum node that the rollups node uses for testing.
# It copies the anvil state from the builder stage and starts the local anvil
# instance.
#
# It also requires the machine-snapshot built in the snapshot-builder stage.
# =============================================================================

FROM devnet-base AS rollups-node-devnet

# Copy anvil state file and devnet deployment info.
ARG DEVNET_BUILD_PATH=/opt/cartesi/rollups-contracts
COPY --from=devnet-deployer ${DEVNET_BUILD_PATH}/anvil_state.json /usr/share/devnet/anvil_state.json
COPY --from=devnet-deployer ${DEVNET_BUILD_PATH}/deployment.json /usr/share/devnet/deployment.json

HEALTHCHECK --interval=1s --timeout=1s --retries=5 \
	CMD curl \
	-X \
	POST \
	-s \
	-H 'Content-Type: application/json' \
	-d '{"jsonrpc":"2.0","id":"1","method":"net_listening","params":[]}' \
	http://127.0.0.1:8545

CMD ["anvil", "--block-time", "1", "--host", "0.0.0.0", "--load-state", "/usr/share/devnet/anvil_state.json"]
