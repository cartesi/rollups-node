// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/idaveconsensus"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

const (
	iorClaimStagingPeriod        uint64 = 30
	iorExternalSettlementAccount uint32 = 9
)

// TestMain starts the node with one-second publisher polls. This interval
// observes several opportunities after protocol readiness and the healthy control.
func observeIORPublicationWindow(ctx context.Context, t testing.TB, check func(context.Context)) {
	t.Helper()
	timer := time.NewTimer(terminalObservationWindow)
	defer timer.Stop()
	ticker := time.NewTicker(terminalObservationInterval)
	defer ticker.Stop()
	for {
		callCtx, cancel := context.WithTimeout(ctx, terminalObservationRPCTimeout)
		check(callCtx)
		cancel()
		select {
		case <-ctx.Done():
			require.NoError(t, ctx.Err(), "publication observation must complete")
		case <-timer.C:
			return
		case <-ticker.C:
		}
	}
}

func (s *TerminalMachineStatesSuite) requireIORPrtState(
	ctx context.Context, consensus *idaveconsensus.IDaveConsensus, epoch *model.Epoch, staged bool,
) {
	s.T().Helper()
	sealed, err := consensus.GetCurrentSealedEpoch(&bind.CallOpts{Context: ctx})
	s.Require().NoError(err)
	s.Require().Zero(sealed.EpochNumber.Cmp(new(big.Int).SetUint64(epoch.Index)),
		"the IOR epoch must not be accepted by the node")
	s.Require().Equal(staged, sealed.IsTournamentResultStaged)
	if staged {
		s.Require().Equal(*epoch.MachineHash, common.Hash(sealed.StagedPostEpochMachineStateHash))
		s.Require().Equal(*epoch.TxBufferDataBlock, common.Hash(sealed.StagedPostEpochOutputsMerkleRoot))
	}
}

func (s *TerminalMachineStatesSuite) requireNoIORStageEvent(
	ctx context.Context, consensus *idaveconsensus.IDaveConsensus, epoch *model.Epoch,
) {
	s.T().Helper()
	events, err := consensus.FilterEpochStaged(&bind.FilterOpts{Context: ctx, Start: epoch.LastBlock},
		[]*big.Int{new(big.Int).SetUint64(epoch.Index)})
	s.Require().NoError(err)
	defer events.Close()
	s.Require().False(events.Next(), "the node must not stage the IOR result")
	s.Require().NoError(events.Error())
}

func (s *TerminalMachineStatesSuite) requireHealthyPrtPublication(ctx context.Context, client *ethclient.Client) {
	s.T().Helper()
	name := uniqueAppName("ior-healthy-control")
	disabled := false
	defer func() {
		if !disabled {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), cliCommandTimeout)
			defer cancel()
			s.Require().NoError(disableApplication(cleanupCtx, name), "disable healthy publication control")
		}
	}()
	// This guest's initial template declares the correct empty root. It only
	// becomes invalid after an input, and this control sends no input.
	_, err := deployApplication(ctx, name,
		envOrDefault("CARTESI_TEST_INVALID_OUTPUTS_ROOT_DAPP_PATH", "applications/invalid-outputs-root-dapp"),
		terminalFixtureSaltFlag, uniqueSalt(), prtFlag,
		claimStagingPeriodFlag, strconv.FormatUint(iorClaimStagingPeriod, 10))
	s.Require().NoError(err)
	tournament := waitForTournamentAndCommitment(ctx, s.T(), s.Require(), name, 0)
	_, err = mineForTournamentTimeout(ctx, client, tournament.Address)
	s.Require().NoError(err)
	waitForTournamentWinner(ctx, s.T(), s.Require(), client, name, 0)
	staged, err := waitForEpochStatus(ctx, s.T(), name, 0, model.EpochStatus_ClaimStaged)
	s.Require().NoError(err)
	s.Require().True(staged.HasMatchingOutputsRoot())
	s.Require().NotNil(staged.StagedAtBlock)
	mineIORAcceptanceBoundary(ctx, s.T(), client, *staged.StagedAtBlock)
	accepted := waitForPrtEpochAcceptedAndBondRecovered(ctx, s.T(), s.Require(), client, name, 0, tournament.Address)
	s.Require().True(accepted.HasMatchingOutputsRoot())
	s.Require().NoError(disableApplication(ctx, name), "disable completed healthy publication control")
	disabled = true
}

func mineIORAcceptanceBoundary(ctx context.Context, t testing.TB, client *ethclient.Client, stagingBlock uint64) {
	t.Helper()
	current, err := client.BlockNumber(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, current, stagingBlock)
	boundary := stagingBlock + iorClaimStagingPeriod
	require.GreaterOrEqual(t, boundary, stagingBlock, "the staging boundary must not overflow")
	if current < boundary {
		gap := boundary - current
		require.LessOrEqual(t, gap, iorClaimStagingPeriod)
		require.NoError(t, anvilMine(ctx, int(gap))) //nolint:gosec // The gap is bounded by the test's 30-block staging period.
	}
}

func iorDaveMachineProof(t testing.TB, epoch *model.Epoch) idaveconsensus.MachineValidityProof {
	t.Helper()
	proof, err := epoch.StateProof()
	require.NoError(t, err)
	return idaveconsensus.MachineValidityProof{
		IflagsYProof:    idaveconsensus.LeafProof{DataBlock: proof.IflagsYDataBlock, Siblings: proof.IflagsYProof},
		HtifTohostProof: idaveconsensus.LeafProof{DataBlock: proof.HtifTohostDataBlock, Siblings: proof.HtifTohostProof},
		TxBufferProof:   idaveconsensus.LeafProof{DataBlock: proof.TxBufferDataBlock, Siblings: proof.TxBufferProof},
	}
}
