ARG FOUNDRY_VERSION=1.4.3
ARG PRT_CONTRACTS_VERSION=3.0.0-alpha.3
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
    amd64) echo "325ba04dc5cb41c110723b00ac291f8269f8cd785028299aad8252ef980961a7  /tmp/foundry.tar.gz" | shasum -a 256 --check ;;
    arm64) echo "209492cb4ebd723d9eac002fa30f41f53c8810105b67d3c32fe8201cf70f89d4  /tmp/foundry.tar.gz" | shasum -a 256 --check ;;
    esac
    tar -zxf /tmp/foundry.tar.gz -C /usr/local/bin
EOF

# Install anvil state file and devnet deployment info.
#
# The release tarball ships UsdWithdrawalOutputBuilderFactory but no per-token
# builder; deploy one against TestFungibleToken at bake time and write a
# synthesized per-contract JSON so the aggregator picks it up alongside the
# shipped deployments.
RUN <<EOF
    set -e
    mkdir -p ${DEVNET_BUILD_PATH}
    URL=https://github.com/cartesi/dave/releases/download
    VERSION=v${PRT_CONTRACTS_VERSION}
    ARTIFACT=cartesi-rollups-prt-${PRT_CONTRACTS_VERSION}-anvil-v${FOUNDRY_VERSION}.tar.gz
    curl -sSL ${URL}/${VERSION}/${ARTIFACT} -o /tmp/contracts.tar.gz
    echo "b18c344964bd0f48b0419f42fddb08956ea958ea34489a3f7c737839eacdebec  /tmp/contracts.tar.gz" | sha256sum --check
    tar -zxf /tmp/contracts.tar.gz -C ${DEVNET_BUILD_PATH}/
    mkdir -p /usr/share/devnet

    # Boot anvil with --state (load + dump combined) on the shipped state.
    # --preserve-historical-states keeps the full block-by-block history on
    # dump; without it anvil prunes historical state and the runtime
    # container would load a shallower state than expected.
    anvil --state ${DEVNET_BUILD_PATH}/state.json \
        --preserve-historical-states --silent &
    ANVIL_PID=$!
    until cast chain-id 2>/dev/null | grep -q 31337; do sleep 0.2; done

    TOKEN=$(jq -r .address ${DEVNET_BUILD_PATH}/deployments/31337/TestFungibleToken.json)
    FACTORY=$(jq -r .address ${DEVNET_BUILD_PATH}/deployments/31337/UsdWithdrawalOutputBuilderFactory.json)
    SALT=0x0000000000000000000000000000000000000000000000000000000000000000

    # Predict the deterministic builder address (used to synth the JSON below).
    BUILDER=$(cast call "$FACTORY" \
        'calculateUsdWithdrawalOutputBuilderAddress(address,bytes32)(address)' \
        "$TOKEN" "$SALT")

    # Deploy the builder via the factory using anvil's first unlocked account.
    DEPLOYER=$(cast rpc eth_accounts | jq -r '.[0]')
    cast send --from "$DEPLOYER" --unlocked "$FACTORY" \
        'newUsdWithdrawalOutputBuilder(address,bytes32)' "$TOKEN" "$SALT"

    # Synthesize the per-contract JSON BEFORE killing anvil, so a cast-send
    # failure (set -e) aborts before we write a JSON unbacked by chain state.
    jq -n --arg addr "$BUILDER" \
        '{contractName: "UsdWithdrawalOutputBuilder", address: $addr}' \
        > ${DEVNET_BUILD_PATH}/deployments/31337/UsdWithdrawalOutputBuilder.json

    # Graceful shutdown lets --state dump the post-deploy chain back to disk.
    # `wait` returns non-zero on a SIGINT-terminated child; `|| true` keeps
    # `set -e` from aborting the build on that expected non-zero.
    kill -INT $ANVIL_PID
    wait $ANVIL_PID || true

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
