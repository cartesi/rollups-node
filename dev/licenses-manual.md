# Third Party Licenses

This file lists the third party components distributed with the Cartesi Rollups Node.
Regenerate it with `make THIRD_PARTY_LICENSES.md` — never edit it directly.

The entries immediately below cover components that are not Go modules and are therefore
invisible to `go-licenses`; they are maintained by hand in `dev/licenses-manual.md`.
Everything after them is generated from the module graph, with the corrections listed in
`dev/licenses-overrides.tsv` applied.

Two components reach users under the GNU Lesser General Public License v3.0: go-ethereum,
statically linked into every binary, and the Cartesi Machine emulator, loaded at run time.
The LGPL-3.0 is written as a set of additional permissions on top of the GPL-3.0. Both texts
are in [licenses/LGPL-3.0.txt](licenses/LGPL-3.0.txt) and
[licenses/GPL-3.0.txt](licenses/GPL-3.0.txt).

The emulator is a dynamic library in its own package. A recipient can replace it without a new
build of the node. go-ethereum is statically linked. The complete source of this project is in
this repository under Apache-2.0. To build the node against a different go-ethereum, add a
`replace` directive for `github.com/ethereum/go-ethereum` to `go.mod` and run `make build`.

The Go bindings under `pkg/contracts/` are generated from the release artifacts of
cartesi/rollups-contracts and cartesi/dave, pinned and checksum-verified by the Makefile. Both
repositories are Apache-2.0. They are listed below for attribution.

## github.com/cartesi/machine-emulator

* Name: github.com/cartesi/machine-emulator
* Version: 0.21.0
* License: [LGPL-3.0](https://github.com/cartesi/machine-emulator/blob/v0.21.0/COPYING)

## github.com/cartesi/rollups-contracts

* Name: github.com/cartesi/rollups-contracts
* Version: 3.0.0-alpha.6
* License: [Apache-2.0](https://github.com/cartesi/rollups-contracts/blob/v3.0.0-alpha.6/LICENSE)
* Note: source of the generated contract bindings under pkg/contracts/.

## github.com/cartesi/dave

* Name: github.com/cartesi/dave
* Version: 3.0.0-alpha.3
* License: [Apache-2.0](https://github.com/cartesi/dave/blob/v3.0.0-alpha.3/LICENSE)
* Note: source of the generated PRT contract bindings under pkg/contracts/.
