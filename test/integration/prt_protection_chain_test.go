// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/itournament"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

// Read the identity and clock from the real contracts, independently of the
// node's projection and the stop reporter.
func prtProtectionEnd(ctx context.Context, t testing.TB, client *ethclient.Client,
	consensusAddress common.Address, epoch uint64,
) (common.Address, uint64) {
	t.Helper()
	consensus, err := idaveconsensus.NewIDaveConsensus(consensusAddress, client)
	require.NoError(t, err)
	events, err := consensus.FilterEpochSealed(&bind.FilterOpts{Context: ctx}, []*big.Int{new(big.Int).SetUint64(epoch)})
	require.NoError(t, err)
	defer func() { require.NoError(t, events.Close()) }()
	var seal *idaveconsensus.IDaveConsensusEpochSealed
	for events.Next() {
		require.Nil(t, seal, "the epoch must have exactly one seal")
		seal = events.Event
	}
	require.NoError(t, events.Error())
	require.NotNil(t, seal, "the stopped epoch must be sealed")
	require.False(t, seal.Raw.Removed)
	root, err := itournament.NewITournament(seal.Tournament, client)
	require.NoError(t, err)
	descriptor, err := root.TournamentDescriptor(&bind.CallOpts{Context: ctx})
	require.NoError(t, err)
	require.Zero(t, descriptor.Level)
	require.Equal(t, seal.Raw.BlockNumber, descriptor.StartInstant)
	require.LessOrEqual(t, descriptor.Allowance, uint64(math.MaxUint64)-descriptor.StartInstant)
	return seal.Tournament, descriptor.StartInstant + descriptor.Allowance
}

// Configuration is additive because SetExpectedLogs replaces its list.
// Wait for required reports before the test's log window ends.
func requirePrtProtectionLogs(ctx context.Context, t testing.TB, logs *LogChecker, expected ...ExpectedLog) {
	t.Helper()
	logs.SetExpectedLogs(t, append(logs.expectedLogs, expected...)...)
	require.NoError(t, pollUntil(ctx, 250*time.Millisecond, func() (bool, error) {
		_, unmatched := scanNodeLogsBetween(t, logs.logStart, time.Now(), expected)
		return len(unmatched) == 0, nil
	}), "emit each required protection-stop report")
}
