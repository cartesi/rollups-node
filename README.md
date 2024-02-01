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

## Dependencies

The Cartesi Node depends on the following Cartesi components:

| Component | Version |
|---|---|
| Cartesi Machine SDK | [v0.16.3](https://github.com/cartesi/machine-emulator-sdk/releases/tag/v0.16.3) |
| Cartesi OpenAPI Interfaces | [v0.7.0](https://github.com/cartesi/openapi-interfaces/releases/tag/v0.7.0) |
| Cartesi Rollups Contracts | [v1.2.0](https://github.com/cartesi/rollups-contracts/releases/tag/v1.2.0) |
| Cartesi Server Manager | [v0.8.3](https://github.com/cartesi/server-manager/releases/tag/v0.8.3) |

## Node Configuration

The node should be configured exclusively with environment variables.
Those variables are described at [Node Configuration](./docs/config.md).

## Node Development

The recommended way of running the node for Cartesi users and application developers is using [Sunodo](https://docs.sunodo.io).
This section of the documentation is for developers that want to modify the node source code.

### Cloning submodules

Before building and running the node, you should download the submodules with the command below.

```sh
make submodules
```

### Generating files

All the necessary auto-generated files are already commited to the project; but if you wish to generate them again, you can run the command below.

```sh
make generate
```

### Building the Docker images

The easiest way to run the node is using the pre-configured Docker images.
These images include a development Ethereum node and a test Cartesi application.
To build those images, run the command below.

```sh
make docker-build
```

### Running the node with Docker

After building the Docker images, you may run the node running the command below.
This command will run a PostgreSQL database and the development Ethereum node.

```sh
make docker-run
```

#### Connecting to Sepolia testnet

It is possible to connect the node to the Sepolia testnet instead of running a local devnet.
First, you need to set the environment variables `RPC_HTTP_URL` and `RPC_WS_URL` with the URLs of the RPC provider.
For instance, if you are using Alchemy, the variables should be set to `https://eth-sepolia.g.alchemy.com/v2/$ALCHEMY_API_KEY` and `wss://eth-sepolia.g.alchemy.com/v2/$ALCHEMY_API_KEY`.
Then, run the command below to run the node.

```sh
make docker-run-sepolia
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
