# Node Development

This page contains instructions for developers who want to run the Rollups Node from source.

## Cloning submodules

Before building and running the Node, you should download the submodules with the command below.

```sh
make submodules
```

## Generating files

All the necessary auto-generated files are already committed to the project, but if you wish to generate them again, you can run the command below.

```sh
make generate
```

## Running with Docker

Running the Node with Docker is the fastest way to get started because it doesn't require you to install other dependencies.

### Building the Docker images

Before running the Node, you must build the required Docker images.
These images include the Rollups Node, an Ethereum development node, and a test Cartesi application.
To build those images, run the command below.

```sh
make docker-build
```

### Running the Node

After building the Docker images, run the command below to start the Node and its external dependencies.

```sh
make docker-run
```

### Connecting to Sepolia testnet

Connecting the Node to the Sepolia testnet is possible instead of running a local Ethereum development node.
First, you need to set the environment variables `RPC_HTTP_URL` and `RPC_WS_URL` with the URLs of the RPC provider.
For instance, if you are using Alchemy, you should set the variables to `https://eth-sepolia.g.alchemy.com/v2/$ALCHEMY_API_KEY` and `wss://eth-sepolia.g.alchemy.com/v2/$ALCHEMY_API_KEY`.
Then, run the command below to start the Node.

```sh
make docker-run-sepolia
```

### Running the distroless node

The distroless image is an image smaller than the regular one, which contains only the node binaries and its dependencies and doesn't come with a shell or package manager.

```sh
make docker-run-distroless
```

## Run Node Natively

This section explains how to run the Rollups Node natively to facilitate development.
We still recommend using Docker to build dependencies such as the Cartesi Machine snapshot and to run external services like the Postgres database and the Ethereum development node.

### Install Dependencies

- [Docker](https://docs.docker.com/engine/install/)
- [Redis](https://redis.io/docs/install/install-redis/)
- [Cartesi Machine](https://github.com/cartesi/machine-emulator)
- [Server Manager](https://github.com/cartesi/server-manager)
- [Golang](https://go.dev/doc/install)
- [Rust](https://www.rust-lang.org/tools/install)

Building the Cartesi Machine and the Server Manager from source is advised.
Make sure you use the same versions used by the Rollups Node and that Server Manager is in your `$PATH`.

The Rollups Node uses the latest versions of Go and Rust.
Make sure these versions are up to date in your system.

### Build Dependencies

Execute the command below to build the Cartesi Machine test snapshot and the devnet.

```sh
make docker-build-deps
```

### Build Rust services

Before running, build the Rust services in the `offchain` directory with the following commands.

```sh
cd ./offchain
cargo build
```

The binaries will be on `./offchain/target/debug`; add this to your `$PATH`.

### Save Machine Snapshot

The Rollups Node needs a Cartesi Machine snapshot of an application.
Use this command to save the snapshot of an echo application to your local file system.

```sh
go run ./cmd/cartesi-rollups-cli/ save-snapshot
```

### Start Dependencies

Once built, start the dependencies.
On a shell, execute the following command:

```sh
go run ./cmd/cartesi-rollups-cli/ run-deps
```

### Run

The Node is configurable via environment variables.
Open another shell and execute the following command to quick-start with default values.

```sh
source ./setup_env.sh
```

Finally, start the Node in the same shell by executing.

```sh
go run ./cmd/cartesi-rollups-node/
```

Optionally, you can build the Go binary and use it directly.

```sh
go build ./cmd/cartesi-rollups-node
./cartesi-rollups-node
```

## Interacting with the Node

The Node repository contains a command-line tool to interact with the Node.
This tool is meant for the Node developers to test and debug it.
This tool is **not** a general-purpose tool to interact with the Cartesi Rollups.
To run the tool, run the command below.

```sh
go run ./cmd/cartesi-rollups-cli
```

The page [`cartesi-rollups-cli`](cli/cartesi-rollups-cli.md) contains the documentation for all available options.
