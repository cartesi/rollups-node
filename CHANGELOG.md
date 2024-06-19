<!-- markdownlint-disable MD024 -->
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Added Rollups end-to-end test using Echo Dapp

### Fixed

- Fixed a bug that caused a `authority_claimer::ClaimMismatch` error when reprocessing inputs after a reboot.

### Changed

- Changed the dispatcher to close epochs based on block numbers instead of block timestamps.
- **BREAKING**: replaced `CARTESI_EPOCH_DURATION` with `CARTESI_EPOCH_LENGTH_IN_BLOCKS` to match the new epoch algorithm, and set its default value to 7200 (1 day worth of blocks, in average, considering one block is mined every 12 seconds).

## [1.4.0] 2024-04-09

### Added

- Added verification to ensure `CARTESI_BLOCKCHAIN_ID` matches the id returned from the Ethereum node
- Added verification to ensure the Cartesi Machine snapshot hash matches the template hash from the CartesiDApp contract
- Added support for `CARTESI_AUTH_PRIVATE_KEY` and `CARTESI_AUTH_PRIVATE_KEY_FILE`
- Added `CARTESI_AUTH_KIND` environment variable to select the blockchain authetication method
- Added structured logging with slog. Colored logs can now be enabled with `CARTESI_LOG_PRETTY` environment variable

### Changed

- Changed `CARTESI_BLOCKCHAIN_ID` type from int to uint64
- Changed `CARTESI_CONTRACTS_APPLICATION_DEPLOYMENT_BLOCK_NUMBER` type from string to int64.
- Changed `CARTESI_LOG_LEVEL` option `warning` to `warn`
- Bumped Cartesi Emulator SDK to 0.17.1
- Bumped Server Manager to 0.9.1

### Removed

- Removed `CARTESI_EXPERIMENTAL_DISABLE_CONFIG_LOG` and `CARTESI_LOG_TIMESTAMP` environment variables

## [1.3.1] 2024-03-13

### Added

- Added `CARTESI_EXPERIMENTAL_SERVER_MANAGER_BYPASS_LOG` env var to allow `server-manager` output to bypass all log configuration
- Added `CARTESI_EXPERIMENTAL_DISABLE_CONFIG_LOG` env var to disable log entries related to the node's configuration

## [1.3.0] 2024-02-09

### Added

- Added `cartesi-rollups-node` Go binary as a single entrypoint to execute all Cartesi Node services
- Added unified configuration for the Node with a new set of environment variables;
  Check `docs/Config.md` for the updated list
- Added `cartesi-rollups-cli` binary to help develop and debug the Cartesi Rollups node
- Added `Makefile` to help node developers setup their environment
- Added experimental sunodo validator mode
- Added instructions on how to run the node with Docker

### Changed

- Changed installation paths in the Docker image. Now all the binaries are installed in `/usr/bin`
  instead of `/opt/cartesi/bin`
- Changed the base Docker image to debian-bookworm instead cartesi/server-manager
- Bumped Cartesi Rollups contracts to version 1.2.0
- Bumped Cartesi Emulator SDK to 0.16.3
- Bumped Server Manager to 0.8.3

### Removed

- Removed concurrent block fetch in foldable `InputBox`
- Removed snapshot-saving feature. Now, the node will always start from the beginning

### Fixed

- Fixed mnemonic leaking through log entries
- Fixed high RPC usage by filtering the "input added" event by the application address

## [1.2.0] 2023-12-04

### Added

- Added `DAPP_ADDRESS`, `DAPP_DEPLOY_BLOCK_HASH`, `HISTORY_ADDRESS`, `AUTHORITY_ADDRESS`, and
  `INPUT_BOX_ADDRESS` env vars to the dispatcher and the authority claimer.
  These are optional and will overwrite the values from the dapp and rollups deployment files.
- Added `authority-claimer` service
- Added `CHAIN_ID` environment variable to dispatcher config
- Added redis-tools to rollups-node Docker image

### Changed

- Added rollups-node version to the logs in all services
- The dispatcher no longer sends claims, this functionality is executed by the authority-claimer
- The dispatcher will now fail to start if there are inputs from a previous execution in the broker
- Bumped Rollups Contracts to 1.1.0
- Bumped Rust Version to 1.74.0

### Removed

- Removed `AUTHORITY` and `TXMANAGER` environment variables from dispatcher config
- Removed deployment files from rollups-node Docker image

## [1.1.0] 2023-10-02

### Added

- Added support to `POST` *inspect state* requests
- Added snapshot validation. The node will now check whether the snapshot's template hash matches the one stored in the blockchain
- Added `PROVIDER_HTTP_ENDPOINT` to advance-runner config
- Added `cartesi/rollups-node` docker image with all node binaries
- Added completion status to GraphQL API

### Changed

- Standardized log libraries and configuration
- Moved GraphQL schema generation to the CI. Now it is distributed as a Github artifact
- Replace `POSTGRES_*` environment variables with `POSTGRES_ENDPOINT`
- Allow negative numbers in `SF_QUERY_LIMIT_ERROR_CODES` state-fold configuration
- Bumped Rust Version to 1.72.1

### Removed

- Removed configurable inspect-server path prefix
- Removed on-chain code from the node repository
- Removed `advance-runner`, `dispatcher`, `graphql-server`, `host-runner`, `inspect-server`, `indexer`, and `state-server` docker images

### Fixed

- Parse `SF_GENESIS_BLOCK` config correctly as decimal
- Setting `SF_CONCURRENT_EVENTS_FETCH` to `0` no longer silently halts the system, and instead use this configuration as *additional* concurrent events, as documented

## [1.0.2] 2023-09-12

### Changed

- Updated state-fold libraries to version 0.8
- Added `SS_MAX_DECODING_MESSAGE_SIZE` to state-server and dispatcher with 100MB default
- Added `MAX_DECODING_MESSAGE_SIZE` to advance runner with 100MB default
- Improved node log for large inputs

### Fixed

- Fixed integer coercion when reading variables in GraphQL API

## [1.0.1] 2023-09-06

### Fixed

- Fixed timestamp in GraphQL API
- Fixed BigInt in GraphQL API

## [1.0.0] 2023-08-22

### Added

- Deployed ERC-1155 portals
- Deployed contracts to mainnets `arbitrum`, `ethereum`, and `optimism`
- Added input relay interface and base contract
- Added `InvalidClaimIndex` error in `History` contract
- Added mainnets to `rollups-cli`
- Added `RPC_URL` environment variable during deployment
- Added Prometheus metrics to the `dispatcher` service
- Added Redis TLS support to the node
- Added Redis cluster support to the node
- Added AWS KMS to sign node transactions
- Added Web Identity support to the AWS KMS signer
- Added README for the node

### Changed

- Started using custom errors in contracts
- Made portals and relays inherit `InputRelay`
- Renamed `inboxInputIndex` to `inputIndex` in contracts
- Deployed contracts deterministically with `CREATE2` factory
- Improved proof generation system for on-chain tests
- Renamed fields in `OutputValidityProof` structure
- Renamed `host-server-manager` service to `host-runner`
- Renamed `deployments` image to `rollups-deployments`
- Standardized health-check in node services
- Standardized node binary names to use `cartesi-node` prefix
- Updated `@cartesi/util` to 6.0.0
- Updated Debian version to bookworm in node Docker images
- Updated Rust version to 1.71.0 in node Docker images
- Updated gRPC interfaces to 0.14.0
- Updated emulator SDK to 0.16.2
- Updated `server-manager` to 0.8.2

### Removed

- Removed base portal and relay contracts and interfaces
- Removed `ConsensusCreated` event from `Authority` contract
- Removed `IInputBox` parameter from `Authority` constructor
- Removed `grpc_health_check` tool from node images
- Removed testnet deployments of `goerli`, `polygon_mumbai`, `bsc_testnet`, `iotex_testnet`, and `chiado`

### Fixed

- Fixed input size limit in `InputBox` contract
- Fixed vouchers and notices proofs in `host-runner`

## [0.9.1] 2023-06-14

### Changed

- Fixed dispatcher to not finish empty epoch

## [0.9.0] 2023-05-19

### Added

- Added support to Cartesi Machine snapshots
- [Script](onchain/rollups/test/foundry/dapp/helper/README.md) for updating proofs used in unit tests
- Authority consensus model implementation (single validator)
- Simple claim storage implementation (one claim per DApp)
- Library that defines several constants related to the canonical off-chain machine
- Added integration tests for proxy
- DApp Address Relay contract (allows the off-chain machine to know the DApp's address)
- Added outputs to rollups events
- Added new indexer that consumes broker events

### Changed

- Simplified the on-chain architecture (not backwards-compatible)
- Adopted [Foundry](https://book.getfoundry.sh/) for contract testing (Hardhat is still being used for deployment)
- `CartesiDApp` does not implement [EIP-2535](https://eips.ethereum.org/EIPS/eip-2535) anymore
- Made each Portal a contract of their own, and shared amongst all the DApps
- Made inputs added by Portals more compact by using the [packed ABI encoding](https://docs.soliditylang.org/en/latest/abi-spec.html#non-standard-packed-mode) instead of the standard one
- Made ERC-20 deposits more generic by allowing base layer transfers to fail, and adding a boolean field signaling whether it was successful or not
- Made ERC-721 deposits more generic by adding an arbitrary data field to be interpreted by the off-chain machine in the execution layer
- Moved the input boxes of every DApp into a single, permissionless contract
- Input boxes are now append-only—they are not cleared every new epoch (old Input Facet)
- Modularized the consensus layer (a DApp can now seamlessly change its consensus model)
- Modularized the claim storage layer (a consensus can now seamlessly change how it stores claims)
- Voucher bitmask position is now determined by the input index (in the input box) and output index
- Validators need now to specify the range of inputs of each claim they submit on-chain
- Refactor GraphQL API for simplified architecture
- Bumped grpc-interfaces to 0.12.0
- Changed inspect-server to use new server-manager interface
- Changed advance-runner to use new server-manager interface

### Removed

- Setup Input
- Quorum consensus model implementation (up to 8 validators)
- Bank contract
- DApp configuration parameters related to the off-chain machine specs (now defined as constants)
- Removed `epochIndex` field from `OutputValidityProof` struct
- Removed headers from inputs added by trusted permissionless contracts like portals and relayers
- Remove polling-based indexer

## [0.8.2] 2023-01-04

### Changed

- Fixed proxy start up after restart
- Changed `sm_pending_inputs_max_retries` default to 600 (retries while polling server-manager for pending inputs)

## [0.8.1] 2022-12-02

### Added

- Support of gnosis chiado testnet

### Changed

- Fixed epoch finalization when running rollups in host mode

## [0.8.0] 2022-11-29

### Added

- Added the rollups-events crate that works as an abstraction to producing and consuming events.
- Added the server-manager-broker-proxy service to consume events from the broker and manage the server-manager.
- Added request id to server-manager calls
- Added server-manager session config as CLI arguments
- Store DApp deployment information in JSON instead of plain text
- Compatibility with networks without EIP1559 transactions

### Changed

- Modified the dispatcher to produce rollups events instead of managing the server-manager.
- Bumped grpc-interfaces to version 0.9.0

## [0.7.0] 2022-11-02

### Changed

- Increase machine deadline
- Minor documentation updates

## [0.6.1] 2022-10-04

### Changed

- Fix dispatcher's configuration for server-manager threads
- Improve documentation of GraphQL API

## [0.6.0] 2022-09-13

### Added

- Deploy to Arbitrum Goerli and Optimism Goerli
- Add queue to serialize concurrent requests in inspect-server

### Changed

- Send inspect-server logs to stdout instead of stderr

## [0.5.0] 2022-08-17

### Added

- Inspect server
- Add path prefix option to inspect server
- Validate notice function to OutputFacet

### Changed

- Remove hardhat-rollups
- Fix indexer to store proofs only when epoch is finished

## [0.4.0] 2022-07-04

### Changed

- Update dependencies to latest emulator SDK with improved exception handling (grpc-interfaces 0.8)

## [0.3.0] 2022-06-14

### Added

- Factory contract to deploy rollups diamond
- Mermaid diagram of the on-chain rollups on README
- Deploy to several testnets (avax_fuji, bsc_testnet, goerli, kovan, polygon_mumbai, rinkeby, ropsten)
- New container with hardhat and deployed contracts for test environment
- New command line tool to deploy DApps

### Changed

- Moved logic from `erc721Deposit` function to `onERC721Received`
- Renamed `ERC721Deposited` event to `ERC721Received` and added `operator` field
- Validators who lost a dispute are removed from the validator set, and cannot redeem fees from previous claims
- Changed the visibility of `Bank`'s state variables to private
- Changed the visibility of `LibClaimsMask`'s functions to internal
- Improved docker entrypoints and configuration
- Gas optimizations

### Deprecated

### Removed

- `erc721Deposit` function (call `safeTransferFrom` from the ERC-721 contract instead)
- `erc20Withdrawal` function call (vouchers now call `transfer` from the ERC-20 contract directly instead)

### Security

## [0.2.0] 2022-04-28

### Added

- FeeManager facet and Bank contract
- Altruistic and Non-altruistic behavior for Validator Node
- Template Hash
- Setup Input
- NFT Portal
- New hardhat tasks

### Changed

- Updated architecture to Diamonds design pattern
- Bumped solc version to 0.8.13
- Separated npm workspaces for `rollups` and `hardhat-rollups`

### Removed

- Specific ERC-20 Portal
- Deprecated mock contracts

<!-- markdownlint-disable MD053 -->
[Unreleased]: https://github.com/cartesi/rollups-node/releases/tag/v1.4.0...HEAD
[1.4.0]: https://github.com/cartesi/rollups-node/releases/tag/v1.4.0
[1.3.1]: https://github.com/cartesi/rollups-node/releases/tag/v1.3.1
[1.3.0]: https://github.com/cartesi/rollups-node/releases/tag/v1.3.0
[1.2.0]: https://github.com/cartesi/rollups-node/releases/tag/v1.2.0
[1.1.0]: https://github.com/cartesi/rollups-node/releases/tag/v1.1.0
[1.0.2]: https://github.com/cartesi/rollups/releases/tag/v1.0.2
[1.0.1]: https://github.com/cartesi/rollups/releases/tag/v1.0.1
[1.0.0]: https://github.com/cartesi/rollups/releases/tag/v1.0.0
[0.9.1]: https://github.com/cartesi/rollups/releases/tag/v0.9.1
[0.9.0]: https://github.com/cartesi/rollups/releases/tag/v0.9.0
[0.8.2]: https://github.com/cartesi/rollups/releases/tag/v0.8.2
[0.8.1]: https://github.com/cartesi/rollups/releases/tag/v0.8.1
[0.8.0]: https://github.com/cartesi/rollups/releases/tag/v0.8.0
[0.7.0]: https://github.com/cartesi/rollups/releases/tag/v0.7.0
[0.6.1]: https://github.com/cartesi/rollups/releases/tag/v0.6.1
[0.6.0]: https://github.com/cartesi/rollups/releases/tag/v0.6.0
[0.5.0]: https://github.com/cartesi/rollups/releases/tag/v0.5.0
[0.4.0]: https://github.com/cartesi/rollups/releases/tag/v0.4.0
[0.3.0]: https://github.com/cartesi/rollups/releases/tag/v0.3.0
[0.2.0]: https://github.com/cartesi/rollups/releases/tag/v0.2.0
[0.1.3]: https://github.com/cartesi/rollups/releases/tag/v0.1.3
[0.1.2]: https://github.com/cartesi/rollups/releases/tag/v0.1.2
[0.1.1]: https://github.com/cartesi/rollups/releases/tag/v0.1.1
[0.1.0]: https://github.com/cartesi/rollups/releases/tag/v0.1.0
