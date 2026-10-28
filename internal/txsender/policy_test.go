// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPolicyHasOnlyOptionalFeeCapAndExistingLegacyMode(t *testing.T) {
	for _, value := range []string{"", "1", "99999999999999999999999999999999"} {
		_, err := NewPolicy(FinancialConfig{MaxFeePerGas: value})
		require.NoError(t, err)
	}
	for _, value := range []string{"0", "-1", "+1", "1.5", " 1", "0x10"} {
		_, err := NewPolicy(FinancialConfig{MaxFeePerGas: value})
		require.Error(t, err)
	}
	tooLarge := new(big.Int).Lsh(big.NewInt(1), 256).String()
	_, err := NewPolicy(FinancialConfig{MaxFeePerGas: tooLarge})
	require.Error(t, err)
	p, err := NewPolicy(FinancialConfig{Legacy: true})
	require.NoError(t, err)
	require.True(t, p.Legacy)
	require.Nil(t, p.MaxFee)
}

func TestPolicyInputsAreCopiedAndCannotChangeAnActiveSender(t *testing.T) {
	b, signer := newFakeBackend(), newFakeSigner(t)
	ceiling := big.NewInt(100)
	s, err := New[int](b, signer, 31337, Policy{MaxFee: ceiling}, func(context.Context) error { return nil }, testLogger())
	require.NoError(t, err)
	ceiling.SetInt64(1)
	_, err = s.PublishNext(t.Context(), newPlanner())
	require.NoError(t, err)
	require.Equal(t, big.NewInt(100), s.policy.MaxFee)
}

func TestLimitErrorNamesResourceAndCopiesAmounts(t *testing.T) {
	required, available := big.NewInt(20), big.NewInt(10)
	err := limitError("signer balance", required, available)
	required.SetInt64(1)
	available.SetInt64(2)
	require.ErrorIs(t, err, ErrLimit)
	require.Contains(t, err.Error(), "signer balance requires 20, available 10")
}
