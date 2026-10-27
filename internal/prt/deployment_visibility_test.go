// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newInitialVisibilityFixture(t *testing.T) (*observerCheckpointFixture, *bytes.Buffer, *int) {
	t.Helper()
	f := newObserverCheckpointFixture(t)
	f.app.LastTournamentCheckBlock = 0
	f.s.deployments = deploymentCache{}
	f.repo.deploymentRows[f.app.ID] = model.DeploymentAdmissionState{ApplicationID: f.app.ID}
	var logs bytes.Buffer
	f.s.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	reads := 0
	f.consensus.deploymentRead = func(_ context.Context, head txsender.BlockRef) (DeploymentEvidence, error) {
		reads++
		return DeploymentEvidence{Head: head, Consensus: f.app.IConsensusAddress,
			Candidate: deploymentNone, Cause: deploymentCauseEmpty}, nil
	}
	return f, &logs, &reads
}

func TestObservationWaitsForInitialDeploymentAtSelectedHead(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Finalized, model.DefaultBlock_Safe, model.DefaultBlock_Latest} {
		t.Run(policy.String(), func(t *testing.T) {
			f, logs, reads := newInitialVisibilityFixture(t)
			f.s.defaultBlock = policy
			before := f.repo.deploymentRows[f.app.ID]
			for head := uint64(100); head < 109; head++ {
				facts := observerFacts(head)
				f.epochs()
				f.consensus.On("GetCurrentSealedEpoch", mock.Anything).Run(func(args mock.Arguments) {
					opts := args.Get(0).(*bind.CallOpts)
					require.Equal(t, facts.Hash, opts.BlockHash)
					require.Nil(t, opts.BlockNumber)
					require.NotNil(t, opts.Context)
					require.False(t, opts.Pending)
				}).Return(CurrentSealedEpoch{}, bind.ErrNoCode).Once()
				require.NoError(t, f.s.observeApplication(t.Context(), f.app, facts))
			}
			require.Zero(t, *reads)
			require.Equal(t, before, f.repo.deploymentRows[f.app.ID], "absence must not acknowledge or clear an episode")
			require.Empty(t, f.s.deployments.entries)
			require.Empty(t, f.s.protectionNotices)
			require.NotContains(t, logs.String(), "level=ERROR")
			require.NotContains(t, logs.String(), "deployment evidence is unavailable")
			facts := observerFacts(109)
			root := common.HexToAddress("0x777")
			f.epochs()
			f.consensus.On("GetCurrentSealedEpoch", protectionOpts(facts)).
				Return(CurrentSealedEpoch{Tournament: root}, nil).Twice()
			adapter := &tournamentAdapterMock{}
			f.factory.On("CreateTournamentAdapter", root).Return(adapter, nil).Once()
			adapter.On("Descriptor", protectionOpts(facts)).Return(TournamentDescriptor{StartInstant: 109, Allowance: 300}, nil).Once()
			f.consensus.deploymentRead = func(_ context.Context, head txsender.BlockRef) (DeploymentEvidence, error) {
				(*reads)++
				return compatibleDeploymentEvidence(head), nil
			}
			require.NoError(t, f.s.observeApplication(t.Context(), f.app, facts))
			require.Equal(t, 1, *reads, "visibility must resume deployment assessment")
			require.Equal(t, int64(1), f.repo.deploymentRows[f.app.ID].ResetGeneration)
			require.Equal(t, deploymentCompatible, f.s.deployments.capture(deploymentIdentity(f.app)).snapshot.Assessment.State)
			require.NotContains(t, logs.String(), "level=ERROR")
			require.Zero(t, f.app.LastTournamentCheckBlock)
			adapter.AssertExpectations(t)
		})
	}
}

func TestObservationInitialVisibilityDoesNotHideRealErrors(t *testing.T) {
	const inventoryErrorCase = "epoch read"
	for _, name := range []string{"transport", "observed cursor", "failed application", inventoryErrorCase} {
		t.Run(name, func(t *testing.T) {
			f, logs, reads := newInitialVisibilityFixture(t)
			facts := observerFacts(100)
			cause := bind.ErrNoCode
			calls := 1
			switch name {
			case "transport":
				cause = errors.New("selected state transport unavailable")
				calls = 2
			case "observed cursor":
				f.app.LastTournamentCheckBlock = 90
			case "failed application":
				f.app.Status = model.ApplicationStatus_Failed
			case inventoryErrorCase:
				// A failed query is not evidence that the application has no obligation.
				queryErr := errors.New("epoch inventory unavailable")
				f.repo.On("ListEpochs", mock.Anything, f.app.Name, repository.EpochFilter{HasTournament: new(true)},
					repository.Pagination{}, false).Return([]*model.Epoch(nil), uint64(0), queryErr).Once()
			}
			if name != inventoryErrorCase {
				f.epochs()
			}
			f.consensus.On("GetCurrentSealedEpoch", protectionOpts(facts)).Return(CurrentSealedEpoch{}, cause).Times(calls)
			err := f.s.observeApplication(t.Context(), f.app, facts)
			if name == inventoryErrorCase {
				require.ErrorContains(t, err, "epoch inventory unavailable")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, *reads, "real failures must continue through assessment and diagnostics")
			require.Contains(t, logs.String(), "level=ERROR msg=\"PRT protection deadline discovery unavailable\"")
			require.Contains(t, logs.String(), cause.Error())
		})
	}
}

func TestObservationMissingCodeKeepsKnownRootDiagnostics(t *testing.T) {
	f, logs, reads := newInitialVisibilityFixture(t)
	facts := observerFacts(100)
	epoch := checkpointEpoch(0, "0x777")
	f.epochs(epoch)
	f.consensus.On("GetCurrentSealedEpoch", protectionOpts(facts)).Return(CurrentSealedEpoch{}, bind.ErrNoCode).Twice()
	adapter := &tournamentAdapterMock{}
	f.factory.On("CreateTournamentAdapter", *epoch.TournamentAddress).Return(adapter, nil).Once()
	adapter.On("Descriptor", protectionOpts(facts)).Return(TournamentDescriptor{}, bind.ErrNoCode).Once()
	require.NoError(t, f.s.observeApplication(t.Context(), f.app, facts))
	require.Equal(t, 1, *reads)
	require.Contains(t, logs.String(), "PRT protection stopped")
	require.Contains(t, logs.String(), "epoch_index=0 root_tournament="+epoch.TournamentAddress.Hex())
	require.Contains(t, logs.String(), "clock_available=false")
	require.Contains(t, logs.String(), bind.ErrNoCode.Error())
	require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
	adapter.AssertExpectations(t)
}
