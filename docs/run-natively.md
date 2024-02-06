# Run Node Natively

This document explains how to run the Rollups Node natively to facilitate development.
We still recommend using Docker to build dependencies such as the Cartesi Machine snapshot and run external ones like the Anvil node with the Cartesi Rollups contracts and the Postgres database.

## Pre-requisites

### Install Dependencies

It is advised to build the Cartesi Machine and the Server Manager from source.
Make sure you use the same versions used by the Rollups Node and that Server Manager is in your `$PATH`.

- [Docker](https://docs.docker.com/engine/install/)
- [Redis](https://redis.io/docs/install/install-redis/)
- [Cartesi Machine](https://github.com/cartesi/machine-emulator)
- [Server Manager](https://github.com/cartesi/server-manager)
- [Golang](https://go.dev/doc/install)
- [Rust](https://www.rust-lang.org/tools/install)

The Rollups Node uses the latest versions of Go and Rust.
Make sure these versions are up to date in your system.

### Build Dependencies

To build the test snapshot and the devnet, execute the command below:

```shell
make docker-build-deps
```

### Save Machine Snapshot

The Rollups Node needs a snapshot of a Cartesi application.
Use this command to save a snapshot of an echo application locally:

```shell
go run ./cmd/cartesi-rollups-cli/ save-snapshot
```

## Build

Before running, build the Rust services in the `offchain` directory with the following commands:

```shell
cd ./offchain
cargo build
```

## Start Dependencies

Once built, start the dependencies.
On a shell, execute the following command:

```shell
go run ./cmd/cartesi-rollups-cli/ run-deps
```

## Run

The Node is configurable via environment variables.
To quick-start with default values, open another shell and execute the following command:

```shell
source ./setup_env.sh
```

Finally, start the Node in the same shell by executing:

```shell
go run ./cmd/cartesi-rollups-node/
```

Optionally, you can build the Go binary and use it directly:

 ```shell
 go build ./cmd/cartesi-rollups-node
 ./cartesi-rollups-node
 ```
