# Cartesi Rollups Node

[![Latest Release](https://img.shields.io/github/v/release/cartesi/rollups-node?label=version)](https://github.com/cartesi/rollups-node/releases)
[![Build Status](https://img.shields.io/github/actions/workflow/status/cartesi/rollups-node/build.yml?branch=main)](https://github.com/cartesi/rollups-node/actions)
[![License](https://img.shields.io/github/license/cartesi/rollups-node)](LICENSE)

The Cartesi Rollups Node is the middleware that connects the Rollups smart contracts, the application back-end running inside the Cartesi Machine, and the front-end.

## Getting Started

### Installation

We provide packages for debian (.deb) in **amd64** and **arm64** variants on the release page. Other systems can build from source.

#### From Sources

##### System Requirements

- Cartesi Machine emulator == 0.22.x
- GNU Make >= 3.81
- Go >= 1.27.3

Follow the [Cartesi Machine installation instructions](https://github.com/cartesi/machine-emulator?tab=readme-ov-file#installation).

##### Build

With the Cartesi machine emulator installed, build the rollups-node:

```sh
# clone a branch of the rollups-node
git clone --branch next/2.0 https://github.com/cartesi/rollups-node.git
cd rollups-node

# compile
make
```

### Install

Optionally setup `GO_INSTALL_PATH`, then:

```sh

# install
sudo make install
```

### Usage

Once installed, the node requires configuration to run.
Via either environment variables or execution arguments.

As an **example**, the development values can be found with `make env`.
Node operators should get familiar with those and customize to their specific setup.
For more on this topic, consult the [wiki](https://github.com/cartesi/rollups-node/wiki/Configuration).
See [Transaction delivery](docs/transaction-delivery.md) for signer configuration, limits, recovery and removal.

The node provides multiple binaries, one for each of its modules (advancer, claimer, evm-reader, validator, jsonrpc-api).
They follow the naming convention `cartesi-rollups-<module>`.
In addition, there is the `cli` tool that provides functionality to help develop and debug the Cartesi Rollups node and
the `node` to run in standalone mode.

Each of these binaries has a help invokable with: `<binary> --help` for discoverability.
For more on this topic, consult the [wiki](https://github.com/cartesi/rollups-node/wiki/Commands).

## Use Cases

The following projects have been using the rollups-node:

- [Cartesi CLI](https://github.com/cartesi/cli) - Uses the emulator's CLI in TypeScript for DApp development.

## Related Projects

TODO

## Benchmarks

TODO

## Documentation

The Cartesi Rollups node documentation is undergoing a comprehensive update.
While the full documentation is being refreshed, you can find guides and tutorials in our [wiki](https://github.com/cartesi/rollups-node/wiki).

## Change Log

Changes between Cartesi Rollups node releases are documented in [CHANGELOG](CHANGELOG).

## Roadmap

We are continually improving the Rollups node with new features and enhancements and ramping up the 2.0 version.
Check out our roadmap at [GitHub Projects](https://github.com/cartesi/rollups-node/projects) to see what's coming in the future.

## Community & Support

- Join our [Discord](https://discord.gg/cartesi) `#node` channel to engage with users and developers.
- Report issues on our [GitHub Issues](https://github.com/cartesi/rollups-node/issues).

## Developing

For information about developing the rollups node, including instructions for running tests, using the linter, and code formatting, please refer to our [development guide](https://github.com/cartesi/rollups-node/wiki/Development-Guide) in the wiki.

## Contributing

Please see our [contributing guidelines](CONTRIBUTING.md) for instructions on how to start contributing to the project.
Note we have a [code of conduct](CODE_OF_CONDUCT.md), please follow it in all your interactions with the project.

## Authors

The Cartesi Rollups Node is actively developed by [Cartesi](https://cartesi.io/)'s Node Reference Unit, with significant contributions from many open-source developers.
For a complete list of authors, see the [AUTHORS](AUTHORS) file.

## License

The project's own code, including all contributions to it, is available under the [Apache License, Version 2.0](LICENSE).
This code is also offered under the [GNU Lesser General Public License, Version 3](licenses/LGPL-3.0.txt), making it dual-licensed: you may use it independently in other applications under either license, at your option.

The distributed node includes LGPL-licensed dependencies.
In particular, go-ethereum is statically linked into every binary, so the node as a whole is subject to LGPL-3.0 when distributed with it.
Only go-ethereum's library packages are used, that is, everything outside `cmd/`.

Third-party components and code covered by separate LICENSE / COPYING files remain subject to their respective licenses.
See [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md) for details.

The Cartesi Machine emulator is a dynamic library in its own package. A recipient can replace it without a new build of the node.
The complete source of this project is in this repository under Apache 2.0. To build the node against a different go-ethereum, add a `replace` directive for `github.com/ethereum/go-ethereum` to `go.mod` and run `make build`.
The full text of the LGPL-3.0 and the GPL-3.0 is in [licenses/](licenses).
