# Cartesi Rollups Node

> [!NOTE]
> This page assumes the reader has a good understanding about Cartesi Rollups.
> The Cartesi Rollups [documentation page][rollups-docs] is an excellent place to learn the concepts related to the framework.

[rollups-docs]: https://docs.cartesi.io/cartesi-rollups/overview/

## Introduction

![Overview](docs/images/overview.svg "Cartesi Rollups Framework Topology")

The diagram above show an overview of the topology of the Cartesi Rollups framework.
The white box on the left represents an EVM-compatible blockchain, such as Ethereum.
The blue boxes are the major Cartesi components used in the framework.
The black boxes are the components of the application built on top of Cartesi Rollups.

The Cartesi Rollups Contracts run in the base layer and are responsible for settlement, consensus, and data availability.
The Cartesi Machine is a deterministic execution environment that runs off-chain.
Both components live in completely decoupled environments, so the framework defines another component to handle the communication between them, the Cartesi Rollups Node (_Node_).

> [!NOTE]
> In this documentation, the term _Node_ is used to refer to the Cartesi Rollups Node.

The Node is the _middleware_ that connects the Cartesi Rollups smart contracts, the application back-end running inside the Cartesi Machine, and the application front-end.
As such, the Node reads `advance-state` inputs from the smart contracts, sends those inputs to the Cartesi Machine, and stores the computation outputs in a database for the application front-end later query them.
The Node also provides a consensus mechanism so users can validate outputs on-chain.

## Features

This section delves into the Node features in more detail.

### Advance state

![Advance State](docs/images/advance.svg "Data flow for `advance-state` inputs")

The diagram above presents the data flow for advancing the rollup state.
First, the application front-end adds an `advance-state` input to the `InputBox` smart contract in the base layer.
The Node watches the events emitted by this contract, reads each associated input and sends it to the application back-end running inside the Cartesi Machine.
Once the application back-end processes the input, the Node stores the resulting outputs and reports in a database.
Finally, the application front-end reads these outputs from the database.

> [!IMPORTANT]
> The Node provides a GraphQL API that may be used by the application front end to read the rollup state.
> This API contains the `advance-state` inputs, the corresponding outputs, and the proofs necessary to validate the outputs on-chain.
> The [`reader.graphql`](api/graphql/reader.graphql) file contains the schema for this API.

### Inspect State

![Inspect State](docs/images/inspect.svg "Data flow for `ispect-state` inputs")

Besides `advance-state` inputs, the Cartesi Machine can also process `inspect-state` inputs.
By definition, these inputs don't alter the machine's state; instead, they are a mechanism to query the application back-end directly.

The application front-end sends `inspect-state` inputs directly to the Node.
The Node passes the inputs to the Cartesi Machine, obtains the reports and returns them to the application front-end (note the `inspect-state` inputs do not return outputs).

The Node provides a REST API for the application front-end to inspect the Cartesi Machine.
Its schema is defined at file [`inspect.yaml`](api/openapi/inspect.yaml).

> [!TIP]
> The Node can process multiple inputs concurrently at a time, being them `advance-state` or `inspect-state`.
> However, the amount of concurrent processess is limited, based upon resources available in the deployment enviroment.
> Make sure to configure the Node accordingly. For more information, refer to the [Configuration](./docs/config.md) page.

> [!CAUTION]
> Be cautious when using `inspect-state` inputs if your application has specific scalability requirements.
> We strongly recommend benchmarking the application back-end before committing to an architecture that relies on `inspect-state` inputs.

### Validate

![Validate](docs/images/validate.svg "Input validation data flow")

The Node bundles multiple `advance-state` inputs into an _epoch_.
Currently, the Node only supports the Authority consensus mechanism, it is the Node that decides when it should close an epoch.
Once an epoch is closed, the Node computes a claim for the epoch and submits it to the Rollups Smart Contracts.
Then, the application front-end fetches the proof corresponding to a given output from a closed epoch and uses it to validate the output.
For instance, the front end may verify whether a notice is valid or execute an output.

Only the validator can submit a claim to the Rollups smart contracts.
The application developer decides who is the validator when deploying the application contract to the base layer.
If you want to run the Node but aren't the application's validator, you may turn off the feature that submits the claims.
The [configuration](#configuration) section describes how to do so.

### Host Mode

The host mode allows the developer to run the application back-end in the host machine instead of the Cartesi Machine.
This feature was deprecated in the Rollups Node 2.0 version; instead, developers should use [NoNodo][nonodo].

[nonodo]: https://github.com/gligneul/nonodo#nonodo

## Running

We recommend application developers to use [Sunodo][sunodo-docs] to run the Node.
Advanced developers who want to modify the Node may check the [development](#development) section.

[sunodo-docs]: https://docs.sunodo.io/

### Configuration

The Node should be configured exclusively with environment variables, which are described at [Node Configuration](docs/config.md) page.

## Dependencies

The Node depends on the following Cartesi components:

| Component | Version |
|---|---|
| Cartesi Machine SDK | [v0.17.1](https://github.com/cartesi/machine-emulator-sdk/releases/tag/v0.17.1) |
| Cartesi OpenAPI Interfaces | [v0.7.1](https://github.com/cartesi/openapi-interfaces/releases/tag/v0.7.1) |
| Cartesi Rollups Contracts | [v1.2.0](https://github.com/cartesi/rollups-contracts/releases/tag/v1.2.0) |
| Cartesi Server Manager | [v0.9.1](https://github.com/cartesi/server-manager/releases/tag/v0.9.1) |

## Development

Check the [development](docs/development.md) page for more information about setting up a local development environment for the Node.

### Internal architecture

This document provides only a high-level overview of features available in the Node and its external APIs.
Check the [architecture](docs/architecture.md) page for more details about the Node internal components.

## Releasing

Check the [release](docs/release.md) page for more information about the steps to release a version of the Node.

## Contributing

Thank you for your interest in Cartesi!
Head over to our [Contributing Guidelines](docs/contributing.md) for instructions on how to sign our Contributors Agreement and get started with Cartesi!

Please note we have a [Code of Conduct](docs/code_of_conduct.md); please follow it in all your interactions with the project.

## License

Note: This component currently has dependencies licensed under the GNU GPL, version 3, so you should treat this component as a whole as being under the GPL version 3.
But all Cartesi-written code in this component is licensed under the Apache License, version 2, or a compatible permissive license, and can be used independently under the Apache v2 license.
After we rewrite this component, we will release it under the Apache v2 license.
