# Run Node Locally Nativelly

This document explains how to run the Rollups Node without docker to facilitate the Node development.
However, we still recommend using Docker to build some dependencies: the Cartesi machine snapshot and the devnet with the Cartesi Rollups contracts.
We also recommend using docker to run some external dependencies: the Anvil node and the Postgres.

## Pre-requisites

### Install Dependencies

It is advised to build the Cartesi Machine and the Server Manager from source.
Make sure you use the same versions the Rollups node uses.

- [Docker](https://docs.docker.com/engine/install/)
- [Redis](https://redis.io/docs/install/install-redis/)
- [Cartesi Machine](https://github.com/cartesi/machine-emulator)
- [Server Manager](https://github.com/cartesi/server-manager)
- [Golang](https://go.dev/doc/install)
- [Rust](https://www.rust-lang.org/tools/install)

The Rollups Node uses the latest versions of Go and Rust.
Make sure these versions are up to date in your system.

### Build Dependencies

To build the test snapshot and the devnet, execute the command below.

```shell
make docker-build-deps
```


### Save Machine Snapshot

The Rollups Node needs a snapshot of a Cartesi application.
To save the snapshot of an echo application locally, open a shell and execute the command below.

```shell
go run ./cmd/cartesi-rollups-cli/ save-snapshot
```

## Build

Before running, build the Node.
Open a shell and execute the commands below.

```shell
cd ./offchain
cargo build
```

## Run

The Node confguration is done through Environment variables.
To quick start with default values open a shell and execute the following command:

```shell
source ./setup_env.sh
```

Once configured, start the dependencies, on a shell start the dependencies with the following command:

```shell
go run ./cmd/cartesi-rollups-cli/ run-deps
```

Finally start the node by opening another shell and executing the node:

```shell
go run ./cmd/cartesi-rollups-node/
```

Optionally you can build the Go binaries and use them:

 ```shell
 go build ./cmd/cartesi-rollups-node
 ./cartesi-rollups-node
 ```
