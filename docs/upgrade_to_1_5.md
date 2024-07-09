<!-- markdownlint-disable MD028 -->
# Upgrade guide for Cartesi Rollups Node `v1.5.0`

Release `v1.5.0` brings a change in the way the Cartesi Rollups Node closes epochs.
They are now closed based on block numbers intead of timestamps.

> [!WARNING]
> This release contains a **BREAKING CHANGE** that fixes issue [#432](https://github.com/cartesi/rollups-node/issues/432), where epochs may be closed wrongly between restarts of the Cartesi Rollups Node, eventually triggering a `ClaimMismatch` error, which causes the Cartesi Rollups Node to abrubtly shut down.

> [!IMPORTANT]
> It's highly recommended that application owners upgrade their instances of the Cartesi Rollups Node to `v1.5.0`, had their applications being affected by the issue above or not.

## What has changed

The way the Cartesi Rollups Node closes epochs has changed starting on `v1.5.0`.
This procedure is now based on the `CARTESI_EPOCH_LENGTH` environment variable instead of `CARTESI_EPOCH_DURATION`.

The value of `CARTESI_EPOCH_LENGTH` (blocks) may be derived from `CARTESI_EPOCH_DURATION` (seconds) as follows:

`CARTESI_EPOCH_LENGTH = CARTESI_EPOCH_DURATION/BLOCK_TIME`

Where `BLOCK_TIME` corresponds to the duration it takes to generate a block in the target network.

> [!TIP]
> Suppose a block is created every `12` seconds and `CARTESI_EPOCH_DURATION` is set to `86400` seconds (24 hours).
>
> So, `CARTESI_EPOCH_LENGTH = 86400/12 = 7200`

Check the [`CHANGELOG`](../CHANGELOG.md) and the [Rollups Node configuration](./config.md) for more details.

## How to upgrade

Application owners may decide to redeploy all necessary contracts and upgrade their instances of the Cartesi Rollups Node to release `v1.5.0`.
This is the simplest way to perform the upgrade.

In order to do so, just update the application configuration considering [what has changed](#what-has-changed) and upgrade the version of the Cartesi Rollups Node as usual.

For more details about how to deploy an application, please refer to the [deployment process](https://docs.cartesi.io/cartesi-rollups/1.3/deployment/introduction/#deployment-process) options available in the Cartesi documentation.

> [!CAUTION]
> A redeployment will create a new instance of the application from scratch.
> All previous inputs, outputs and claims will remain associated to the previous application address, including any funds locked in the application contract.

Alternatively, the _Authority_ owner may choose to replace the _History_ being used by the application's _Authority_.
This process allows inputs, outputs and locked funds to remain unchanged, but is a little bit more involved.
It is described in the next section.

> [!NOTE]
> A new _History_ will contain no claims.
> Once the Rollups Node is restarted with a new _History_, all previous claims will be submitted again, one-by-one, by the configured _Authority_, incurring in extra cost for the _Authority_ owner.

## Steps to replace an Application's _History_

> [!CAUTION]
> Instances of the [History](https://github.com/cartesi/rollups-contracts/blob/v1.2.0/onchain/rollups/contracts/history/History.sol) contract from [rollups-contracts v1.2.0](https://github.com/cartesi/rollups-contracts/releases/tag/v1.2.0) may be used simultaneously by several applications through their associated _Authority_ instance.
> Application owners must consider that and exercise care when performing the steps listed below.

To keep the application inputs, before performing the upgrade to version `v1.5.0`, proceed as described in the sub-sections below.

### 1. Instantiate a new _History_

This is a two-step process.
First, the address of the new _History_ instance should be calculated.

> [!NOTE]
> It's recommended to use the deterministic deployment functions available in the Rollups Contracts.

> [!IMPORTANT]
> All commands below assume that the following environment variables have been previous defined:
>
> - `SALT`: a random 32-byte value to be used by the deterministic deployment functions
> - `RPC_URL`: the RPC endpoint to be used
> - `MNEMONIC`: mnemonic phrase for the _Authority_ owner's wallet (other wallet options may be used)
> - `HISTORY_FACTORY_ADDRESS`: address of a valid _HistoryFactory_ instance
> - `AUTHORITY_ADDRESS`: address of the _Authority_ instance being used by the application

> [!TIP]
> A _HistoryFactory_ is deployed at `0x1f158b5320BBf677FdA89F9a438df99BbE560A26` for all supported networks (Ethereum, Optimism, Arbitrum, Base and their respective Sepolia-based tesnets).

In order to calculate the address of a new history contract, one can call the view function [`calculateHistoryAddress(address,bytes32)`](https://github.com/cartesi/rollups-contracts/blob/e8a2d82bc51167086e7928b9a6aced0d62c96cf8/onchain/rollups/contracts/history/HistoryFactory.sol#L35-L38).
This can be done with the the help of Foundry's [cast](https://book.getfoundry.sh/reference/cast/) as follows:

```shell
cast call \
    --trace --verbose \
    $HISTORY_FACTORY_ADDRESS \
    "calculateHistoryAddress(address,bytes32)(address)" \
    $AUTHORITY_ADDRESS \
    $SALT \
    --rpc-url "$RPC_URL"
```

Assuming the command execution is successfull, it should print out the address of the to-be-created History contract. You may store this address in the environment variable `NEW_HISTORY_ADDRESS`, to be used later on.

After that, the new instance of _History_ may be created by calling function [`newHistory(address,bytes32)`](https://github.com/cartesi/rollups-contracts/blob/e8a2d82bc51167086e7928b9a6aced0d62c96cf8/onchain/rollups/contracts/history/HistoryFactory.sol#L24-L27) as follows:

```shell
cast send \
    --json \
    --mnemonic "$MNEMONIC" \
    $HISTORY_FACTORY_ADDRESS \
    "newHistory(address,bytes32)(History)" \
    $AUTHORITY_ADDRESS \
    $SALT \
    --rpc-url "$RPC_URL"
```

> [!NOTE]
> `cast send` will fail if the `History` type is not known to `cast` at the time of the execution.
> In such case, it's safe to replace `History` with `address` as return type to `newHistory()` and perform the call again.

> [!NOTE]
> A `cast send` command may fail due to a gas [estimation failure](https://github.com/foundry-rs/foundry/issues/3093#issuecomment-1251616792), which can be circumvented by providing gas constraints to the command with parameter `gas-limit` (e.g.,`--gas-limit 7000000`).

### 2. Replace the _History_

> [!IMPORTANT]
> The command below assumes that the environment variables set in the previous step are available, as well as  `NEW_HISTORY_ADDRESS`, which must contain the address of the new _History_.

To replace the _History_ used by the _Authority_, call [`setHistory(address)`](https://github.com/cartesi/rollups-contracts/blob/e8a2d82bc51167086e7928b9a6aced0d62c96cf8/onchain/rollups/contracts/consensus/authority/Authority.sol#L65) as follows:

```shell
cast send \
    --json \
    --mnemonic "$MNEMONIC" \
    "$AUTHORITY_ADDRESS" \
    "setHistory(address)" \
    "$NEW_HISTORY_ADDRESS" \
    --rpc-url "$RPC_URL"
```

After the _History_ replacement is complete, one must update `CARTESI_CONTRACTS_HISTORY_ADDRESS` with the new _History_ address in the application configuration and proceed with the upgrade of the Cartesi Rollups Node as usual.
More details about the release may be found in the [CHANGELOG](../CHANGELOG.md).

Once the Cartesi Rollups Node is started, it will relay all existing inputs to be processed by the application, calculate the new epochs and send the respective claims to the new _History_ as a result, all based on the updated configuration.
