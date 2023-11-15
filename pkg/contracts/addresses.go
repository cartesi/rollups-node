// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package contracts

import "github.com/ethereum/go-ethereum/common"

var (
	InputBoxAddress common.Address
)

func init() {
	InputBoxAddress = common.HexToAddress("0x59b22D57D4f067708AB0c00552767405926dc768")
}
