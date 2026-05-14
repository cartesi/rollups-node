// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import "github.com/ethereum/go-ethereum/common"

func hashToHex(h *common.Hash) string {
	if h == nil {
		return ""
	}
	return h.Hex()
}
