# Cartesi Node Reference Implementation

The [Cartesi Node](https://docs.cartesi.io/cartesi-rollups/main-concepts/#cartesi-nodes) is the part of the [Cartesi Rollups Framework](https://docs.cartesi.io/cartesi-rollups/overview/) that is responsible for handling the communication between the on-chain smart contracts and the [Cartesi Machine](https://docs.cartesi.io/machine/intro/).

The Cartesi Rollups machine and smart contracts live in fundamentally different environments.
This creates the need for the node that manages and controls the communication between the blockchain and the machine.
As such, the node is responsible for first reading data from Cartesi smart contracts, then sending them to the machine to be processed, and finally publishing their results back to the blockchain.

The node can be used by anyone who's interested in the rollups state of affairs.
We divide interested users into two roles, which run different types of nodes: readers and validators.

Reader nodes are only interested in advancing their off-chain machine.
They consume information from the blockchain but do not bother to enforce state updates, trusting that validators will ensure the validity of all on-chain state updates.

Validators, on the other hand, have more responsibility: they not only watch the blockchain but also fight to ensure that the blockchain will only accept valid state updates.

## Getting Started

### Cloning submodules

Before building and running any of the inner projects, you should download the submodules with:

```shell
git submodule update --init --recursive
```

### Building the Docker image

For more information on how to build the rollups-node docker image, see the [build directory](./build/README.md).

### Rust

All projects comprising the Cartesi Node require Rust to be executed.
To install it, follow the [instructions](https://www.rust-lang.org/tools/install) from the Rust website.

### Dependencies

The Cartesi Node depends on the following components:

| Component | Version |
|---|---|
| Cartesi Machine SDK | [v0.16.2](https://github.com/cartesi/machine-emulator/releases/tag/v0.16.2) |
| Cartesi OpenAPI Interfaces | [v0.6.0](https://github.com/cartesi/openapi-interfaces/releases/tag/v0.6.0) |
| Cartesi Rollups Contracts | [v1.1.0](https://github.com/cartesi/rollups-contracts/releases/tag/v1.1.0) |
| Cartesi Server Manager | [v0.8.2](https://github.com/cartesi/server-manager/releases/tag/v0.8.2) |

### Running

To run any of the inner projects, execute the command:

```shell
cargo run
```

Some of the inner projects may have additional run instructions.
Refer to their own documentation for more details.

## Configuration

It is possible to configure the behavior of any of the projects by passing CLI arguments and using environment variables.
Execute the following command to check the available options for each project:

```shell
cargo run -- -h
```

### Redis TLS Configuration

To connect the Broker to a Redis server via TLS, the server's URL must use the `rediss://` scheme (with two "s"es).
This is currently the only way to tell `Broker` to use a TLS connection.

## Tests

To run the tests available in any of the projects, execute the command:

```shell
cargo test
```

## Architecture Overview

The Cartesi Node is an event-based solution that runs the following microservices:

The **Broker** is a Redis-based message broker that mediates the transferring of *Inputs*, *Outputs* and *Claims* between the Cartesi Node components.
For details specific to the Broker and the available event streams, refer to the [Rollups Events project](./offchain/rollups-events/README.md).

The [**State-fold Server**](./offchain/state-server/README.md) is the component responsible for tracking blockchain state changes and deriving *Inputs* from them to be processed.

The [**Dispatcher**](./offchain/dispatcher/README.md) is the component that messages *Inputs* via the **Broker** to be processed later on.

The [**Authority Claimer**](./offchain/authority-claimer/README.md) is the component that submits *Claims* to the blockchain at the end of each epoch that contains at least one *Input*.

The [**Advance Runner**](./offchain/advance-runner/README.md) is the one responsible for relaying *Inputs* to the **Server Manager** to be processed by the underlying DApp running in the embedded **Cartesi Machine**.
The **Advance Runner** obtains the resulting **Outputs** and **Claims** from the **Server Manager** and adds them to the **Broker**.

The [**Indexer**](./offchain/indexer/README.md) consumes all *Inputs* and *Outputs* transferred by the **Broker** and stores them in a **PostgreSQL** database for later querying via a **GraphQL Server** instance.

The [**Inspect Server**](./offchain/inspect-server/README.md) is responsible for the processing of *Inspect* requests via HTTP.

## Rollups Node Features

The Cartesi Node contains 3 main features, the combination of which defines the node capabilities.

### Input Processing

The Node is capable of processing *Inputs* stored in the blockchain.
To do that, the node retrieves them from the blockchain, sends them to the Cartesi Machine, and stores the corresponding results in the node database.
Once an input result is in the database, the node client can query the respective outputs in the GraphQL API.

The *Input* is read from the blockchain by the **State-fold Server**, is then received by the **Dispatcher** and relayed to the **Broker** through an input-specific stream.
The *Input* is eventually consumed by the **Advance Runner** and used to advance the state of the **Server Manager**, thus generating an *Advance* request to be processed by the underlying DApp.

After finishing the processing, the DApp may generate a number of *Outputs*, which are eventually retrieved by the **Advance Runner** and fed back into the **Broker** through an output-specific stream.

*Inputs* and *Outputs* are consumed from their respective streams by the **Indexer** and stored in a **PostgreSQL** database and may be queried by the DApp **Frontend** via a **GraphQL Server** instance.

### Claiming

Claiming allows a node to be run as a *Validator* node.
It complements the input processing by generating *Claims* at the end of *Epochs* that have *Inputs*, which are generated by the **Server Manager**.
They are then sent to the **Broker** by the **Advance Runner** through a claims-specific stream to be eventually consumed by the **Authority Claimer** for being submitted to the blockchain.

### Inspecting Data

Every *Inspect* request sent by the **Frontend** via the Rollups HTTP API is captured by the **Inspect Server** and forwarded to the **Server Manager**, which queries the state of the underlying DApp via *Inspect* requests.

Every request results in an *Inspect Response* that is returned to the **Inspect Server**, which sends it back to the **Frontend**.

## Execution modes

The Cartesi Node may act as a *Reader*, a *Validator* or run locally as a *Host*, performing different roles, presented next.

### Reader Mode

The *Reader Mode* connects the node, however it doesn't submit claims to the blockchain, instead only reads and updates the state.
It processes the input and provides the inspect API for queries.

![Reader mode architecture diagram](./docs/reader-architecture.drawio.svg)

To run the node in *Reader Mode*, one must have a Redis instance to act as the *Broker*, a PostgreSQL database instance and a GraphQL instance, and run the *State-fold Server*, *Dispatcher*, *Advance Runner*, *Indexer* and *Inspect Server*.

### Validator Mode

The *Validator Mode* has all the features of the *Reader Mode*, but can also submit claims through the [*Claiming Feature*](#claiming).

![Validator mode architecture diagram](./docs/node-architecture.drawio.svg)

To run the node in *Validator Mode*, one must start the services as described in the *Reader Mode* above, but also initiate the *Authority Claimer* service.

### Host Mode

The Cartesi Node may operate in a so-called *Host mode* when deployed locally for development purposes.

In this case, the overall topology is very similar to the one presented in the [Validator Mode](#validator-mode) as depicted below.

![Host mode architecture diagram](./docs/host-architecture.drawio.svg)

The main difference is that the **Server Manager** is not present and, there's no **Cartesi Machine** available as a result.
Their features are emulated by the **Host Runner**, which interacts with the **DApp Backend** being executed locally in the *host* to perform the Cartesi Node.

## Contributing

Thank you for your interest in Cartesi! Head over to our [Contributing Guidelines](CONTRIBUTING.md) for instructions on how to sign our Contributors Agreement and get started with Cartesi!

Please note we have a [Code of Conduct](CODE_OF_CONDUCT.md), please follow it in all your interactions with the project.

## License

Note: This component currently has dependencies that are licensed under the GNU GPL, version 3, and so you should treat this component as a whole as being under the GPL version 3. But all Cartesi-written code in this component is licensed under the Apache License, version 2, or a compatible permissive license, and can be used independently under the Apache v2 license. After this component is rewritten, the entire component will be released under the Apache v2 license.
