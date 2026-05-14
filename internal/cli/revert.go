// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package cli

import (
	"fmt"

	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

// DecorateRevert wraps a transaction error with a human-readable description
// of its custom-error revert data, decoded against the given contract ABIs.
// Without it the operator sees only the raw selector hex. Returns err
// unchanged when there is no revert data or no ABI declares the selector.
func DecorateRevert(err error, metadatas ...*bind.MetaData) error {
	if err == nil {
		return nil
	}
	if desc, ok := ethutil.DescribeRevert(err, metadatas...); ok {
		return fmt.Errorf("%w\ndecoded revert: %s", err, desc)
	}
	return err
}
