// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"fmt"
	"math/big"
)

// LimitError identifies the refused resource in its natural units: wei, gas,
// bytes or item count. Required and Available are independent amount snapshots.
type LimitError struct {
	Limit               string
	Required, Available *big.Int
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("%s: %s requires %s, available %s", ErrLimit, e.Limit, e.Required, e.Available)
}

func (*LimitError) Unwrap() error { return ErrLimit }

func limitError(name string, required, available *big.Int) error {
	return &LimitError{Limit: name, Required: copyInt(required), Available: copyInt(available)}
}

func countLimit(name string, required, available int) error {
	return limitError(name, big.NewInt(int64(required)), big.NewInt(int64(available)))
}

func gasLimit(name string, required, available uint64) error {
	return limitError(name, new(big.Int).SetUint64(required), new(big.Int).SetUint64(available))
}
