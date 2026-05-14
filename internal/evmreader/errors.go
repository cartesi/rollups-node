// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package evmreader

import "errors"

var errContractNotDeployedAtBlock = errors.New("contract not deployed at block")
