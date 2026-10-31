// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package validator

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/repository"
)

func TestOutputChunkScanPreservesCallbackError(t *testing.T) {
	sentinel := errors.New("builder failed")
	err := scanOutputChunk(t.Context(), 0, 0, func(_ context.Context, first, _ uint64, fn repository.OutputScanFunc) error {
		return fn(first, []byte("row"))
	}, func(common.Hash) error { return sentinel })
	require.ErrorIs(t, err, sentinel)
}

func TestOutputChunkScanClassifiesMissingAndExtraRows(t *testing.T) {
	for _, indexes := range [][]uint64{nil, {0}, {0, 0}, {1}, {0, 1, 2}} {
		err := scanOutputChunk(t.Context(), 0, 1, func(_ context.Context, _, _ uint64, fn repository.OutputScanFunc) error {
			for _, index := range indexes {
				if err := fn(index, []byte("row")); err != nil {
					return err
				}
			}
			return nil
		}, func(common.Hash) error { return nil })
		require.ErrorIs(t, err, merkle.ErrOutputsChanged)
	}
}
