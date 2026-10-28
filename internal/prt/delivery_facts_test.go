// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"fmt"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
)

func TestFinalizedFactsRejectUnvalidatedCandidateWithoutPoisoningWatermark(t *testing.T) {
	f, client := tickFixture(t)
	tickHead(client, 100)
	first, err := f.d.observationReference(t.Context())
	require.NoError(t, err)
	inflated := &types.Header{Number: big.NewInt(1_000_000)}
	client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).Return(inflated, nil).Once()
	// The ordinary numeric response disagrees with the single bad tag reply.
	inflated.Extra = []byte("unvalidated tag reply")
	_, err = f.d.observationReference(t.Context())
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.Equal(t, first, f.d.observedFacts)
	tickHead(client, 101)
	last, err := f.d.observationReference(t.Context())
	require.NoError(t, err)
	require.Equal(t, uint64(101), last.Number)
}

func TestFinalizedFactsWaitOnRegressionAndStopOnConfirmedContradiction(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprint(conflict), func(t *testing.T) {
			f := newDeliveryFixture(t)
			f.d.defaultBlock = model.DefaultBlock_Finalized
			client := &ethClientMock{}
			f.d.client = client
			header := &types.Header{Number: big.NewInt(100)}
			client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).Return(header, nil).Once()
			client.On("HeaderByNumber", mock.Anything, big.NewInt(100)).Return(header, nil).Once()
			first, err := f.d.observationReference(t.Context())
			require.NoError(t, err)
			older := &types.Header{Number: big.NewInt(90)}
			client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).Return(older, nil).Once()
			client.On("HeaderByNumber", mock.Anything, big.NewInt(90)).Return(older, nil).Once()
			_, err = f.d.observationReference(t.Context())
			require.ErrorIs(t, err, txsender.ErrInconclusive)
			require.Equal(t, first, f.d.observedFacts)
			next := &types.Header{Number: big.NewInt(101)}
			previous := types.CopyHeader(header)
			if conflict {
				previous.Extra = []byte("contradictory finalized branch")
			}
			client.On("HeaderByNumber", mock.Anything, big.NewInt(rpc.FinalizedBlockNumber.Int64())).Return(next, nil).Once()
			client.On("HeaderByNumber", mock.Anything, big.NewInt(101)).Return(next, nil).Once()
			client.On("HeaderByNumber", mock.Anything, big.NewInt(100)).Return(previous, nil).Once()
			_, err = f.d.observationReference(t.Context())
			if conflict {
				require.ErrorIs(t, err, txsender.ErrFinality)
				_, err = f.d.observationReference(t.Context())
				require.ErrorIs(t, err, txsender.ErrFinality, "a confirmed finalized contradiction cannot be demoted")
			} else {
				require.NoError(t, err)
			}
			client.AssertExpectations(t)
		})
	}
}

func TestProtectedReadFailureRetriesWithoutChangingProtocolState(t *testing.T) {
	f, client := tickFixture(t)
	f.backend.headerError = txsender.ErrUnsupported
	for range 2 {
		tickHead(client, 100)
		require.ErrorIs(t, f.d.tick(t.Context()), txsender.ErrUnsupported)
	}
	require.False(t, f.d.publicationStopped())
	require.Len(t, f.backend.headerReads, 2)
	require.Empty(t, f.backend.published)
	f.backend.headerError = nil
	f.repo.apps = nil
	tickHead(client, 100)
	require.NoError(t, f.d.tick(t.Context()))
	require.Empty(t, f.backend.published)
}

func TestObservedComputedEpochIncompleteProofMarksCorrupted(t *testing.T) {
	for _, part := range []string{"transaction buffer", "iflags", "htif"} {
		t.Run(part, func(t *testing.T) {
			f := newDeliveryFixture(t)
			view := resultTestSnapshot(f.epoch, false)
			switch part {
			case "transaction buffer":
				f.epoch.TxBufferProof = nil
			case "iflags":
				f.epoch.IflagsYDataBlock = nil
			case "htif":
				f.epoch.HtifTohostProof = f.epoch.HtifTohostProof[:model.StateProofSiblingCount-1]
			}
			f.repo.On("UpdateApplicationStatus", mock.Anything, f.app.ID, model.ApplicationStatus_Corrupted,
				mock.Anything).Return(nil).Once()
			err := f.d.recordTournamentResult(t.Context(), f.app, f.epoch, view)
			require.ErrorContains(t, err, "persisted machine state proof is incomplete")
			require.Equal(t, model.ApplicationStatus_Corrupted, f.app.Status)
			f.repo.AssertExpectations(t)
		})
	}
}
