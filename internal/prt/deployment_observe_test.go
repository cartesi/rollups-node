// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestDeploymentObserverReadsOutsideCacheLockAndRetainsKnownShape(t *testing.T) {
	service := newDeploymentObserverService()
	service.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	app := prtRevertTestApp()
	head := deploymentTestHead(100)
	cause := errors.New("late row unavailable")
	calls := 0
	consensus := &daveConsensusAdapterMock{
		deploymentRead: func(_ context.Context, requested txsender.BlockRef) (DeploymentEvidence, error) {
			calls++
			unlocked := service.deployments.mu.TryLock()
			if unlocked {
				service.deployments.mu.Unlock()
			}
			require.True(t, unlocked, "RPC was called while the publication lock was held")
			evidence := tableDeploymentEvidence(requested)
			evidence.Candidate = deploymentNone
			evidence.Rows = evidence.Rows[:1]
			return evidence, cause
		}}
	snapshot, err := service.observeDeployment(t.Context(), app, consensus, head)
	require.ErrorIs(t, err, cause)
	require.Equal(t, 1, calls)
	require.Equal(t, deploymentNone, snapshot.Assessment.State)
	require.True(t, snapshot.Alpha6Shape)
	consensus.deploymentRead = func(_ context.Context, requested txsender.BlockRef) (DeploymentEvidence, error) {
		return DeploymentEvidence{Head: requested, Consensus: app.IConsensusAddress, Candidate: deploymentNone}, txsender.ErrUnsupported
	}
	next, err := service.observeDeployment(t.Context(), app, consensus, deploymentTestHead(101))
	require.ErrorIs(t, err, txsender.ErrUnsupported)
	require.True(t, next.Alpha6Shape)
	require.Equal(t, deploymentNone, next.Assessment.State)
	require.False(t, service.publicationStopped())
}

func TestDeploymentObserverConfirmsOnlyCanonicalDistinctHeads(t *testing.T) {
	for _, canonical := range []bool{true, false} {
		t.Run(map[bool]string{true: "canonical", false: "reorged first head"}[canonical], func(t *testing.T) {
			service := newDeploymentObserverService()
			service.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			app := prtRevertTestApp()
			oldHeader := &types.Header{Number: big.NewInt(100)}
			newHeader := &types.Header{Number: big.NewInt(101)}
			first := txsender.BlockRef{Number: 100, Hash: oldHeader.Hash()}
			second := txsender.BlockRef{Number: 101, Hash: newHeader.Hash()}
			client := &ethClientMock{}
			service.client = client
			returned := types.CopyHeader(oldHeader)
			if !canonical {
				returned.Extra = []byte("another branch")
			}
			client.On("HeaderByNumber", mock.Anything, big.NewInt(100)).Return(returned, nil).Once()
			reads := 0
			consensus := &daveConsensusAdapterMock{
				deploymentRead: func(_ context.Context, h txsender.BlockRef) (DeploymentEvidence, error) {
					reads++
					return tableDeploymentEvidence(h), nil
				}}
			snapshot, err := service.observeDeployment(t.Context(), app, consensus, first)
			require.NoError(t, err)
			require.Equal(t, deploymentPending, snapshot.Assessment.State)
			same, err := service.observeDeployment(t.Context(), app, consensus, first)
			require.NoError(t, err)
			require.Equal(t, deploymentPending, same.Assessment.State)
			final, err := service.observeDeployment(t.Context(), app, consensus, second)
			require.NoError(t, err)
			expected := deploymentPending
			if canonical {
				expected = deploymentTable
			}
			require.Equal(t, expected, final.Assessment.State)
			if canonical {
				_, err = service.observeDeployment(t.Context(), app, consensus, deploymentTestHead(102))
				require.NoError(t, err)
				require.Equal(t, 3, reads)
			}
			client.AssertExpectations(t)
		})
	}
}

func TestDeploymentObservationNeverBecomesWholeApplicationTierGate(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending old shape", true: "confirmed generation"}[confirmed], func(t *testing.T) {
			f := newDeliveryIntervalFixture(t)
			var logs bytes.Buffer
			f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
			f.repo.publishedRoots = nil
			f.checkpoint.epochs(f.epoch)
			f.block = f.facts
			f.snapshot(resultTestSnapshot(f.epoch, false))
			expectProtectionReads(t, f.factory, f.consensus, f.facts, f.epoch)
			reads := 0
			f.consensus.deploymentRead = func(_ context.Context, h txsender.BlockRef) (DeploymentEvidence, error) {
				reads++
				e := compatibleDeploymentEvidence(h)
				e.Candidate = deploymentGeneration
				e.Cause = deploymentCauseStaticShape
				e.Alpha6Shape = false
				e.Rows = nil
				return e, nil
			}
			if confirmed {
				key := deploymentIdentity(f.app)
				snapshot := deploymentSnapshot{Acknowledged: true, ResetGeneration: 1,
					Assessment: deploymentAssessment{State: deploymentGeneration}}
				f.d.deployments.publish(key, snapshot)
			}
			require.NoError(t, f.d.observeApplication(t.Context(), f.app, f.facts))
			f.consensus.AssertCalled(t, "CanStageTournamentResult", mock.Anything)
			f.consensus.AssertCalled(t, "CanAcceptStagedTournamentResult", mock.Anything)
			f.consensus.AssertNotCalled(t, "TournamentLevelCount", mock.Anything)
			require.Empty(t, f.repo.windows)
			require.Equal(t, model.ApplicationStatus_OK, f.app.Status)
			require.False(t, f.d.publicationStopped())
			require.Contains(t, logs.String(), "join_close_block=310")
			require.Contains(t, logs.String(), "clock_available=true")
			expected := 1
			if confirmed {
				expected = 0
			}
			require.Equal(t, expected, reads)
		})
	}
}

func TestDeploymentObservationSkipsDisabledApplicationBeforeRPC(t *testing.T) {
	f := newDeliveryFixture(t)
	f.app.Enabled = false
	before := f.d.deployments.capture(deploymentIdentity(f.app))
	require.NoError(t, f.d.observeApplication(t.Context(), f.app, f.block))
	require.True(t, f.d.deployments.same(deploymentIdentity(f.app), before))
	f.factory.AssertNotCalled(t, "CreateDaveConsensusAdapter", mock.Anything)
}

func TestDeploymentObserverMissingDependencyWarnsWithoutVerdict(t *testing.T) {
	for _, missing := range []string{"factory", "transition"} {
		t.Run(missing, func(t *testing.T) {
			adapter, b := newDeploymentViewRPC(t, canonicalDeploymentRows(), 2)
			if missing == "factory" {
				b.absent[b.factory] = true
			} else {
				b.absent[b.transition] = true
			}
			service := newDeploymentObserverService()
			var logs bytes.Buffer
			service.Logger = slog.New(slog.NewTextHandler(&logs, nil))
			app := prtRevertTestApp()
			app.IConsensusAddress = b.consensus
			snapshot, err := service.observeDeployment(t.Context(), app, adapter, b.head)
			require.NoError(t, err)
			require.Equal(t, deploymentNone, snapshot.Assessment.State)
			require.Contains(t, logs.String(), "level=WARN")
			require.Contains(t, logs.String(), "reason=empty_return")
			require.Contains(t, logs.String(), b.factory.Hex())
			require.False(t, service.publicationStopped())
			require.Equal(t, model.ApplicationStatus_OK, app.Status)
		})
	}
}

func TestDeploymentObserverCanonicalReadErrorKeepsThePendingPair(t *testing.T) {
	service := newDeploymentObserverService()
	service.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	app := prtRevertTestApp()
	firstHeader := &types.Header{Number: big.NewInt(100)}
	head := txsender.BlockRef{Number: 100, Hash: firstHeader.Hash()}
	client := &ethClientMock{}
	service.client = client
	cause := errors.New("historical header unavailable")
	client.On("HeaderByNumber", mock.Anything, big.NewInt(100)).Return((*types.Header)(nil), cause).Once()
	reader := &daveConsensusAdapterMock{deploymentRead: func(_ context.Context, h txsender.BlockRef) (DeploymentEvidence, error) {
		return tableDeploymentEvidence(h), nil
	}}
	first, err := service.observeDeployment(t.Context(), app, reader, head)
	require.NoError(t, err)
	next, err := service.observeDeployment(t.Context(), app, reader, deploymentTestHead(101))
	require.ErrorIs(t, err, cause)
	require.Equal(t, first.Assessment, next.Assessment)
	require.Equal(t, first.Assessment.PendingEvidence, next.Assessment.PendingEvidence)
	require.Equal(t, deploymentPending, next.Assessment.State)
	client.AssertExpectations(t)
}

func TestDeploymentObserverConfirmsActualGenerationAndTableEvidence(t *testing.T) {
	const wantPending = "pending"
	for _, tc := range []struct {
		name      string
		configure func(*deploymentViewRPC)
		want      string
	}{
		{
			name: "old static shape",
			configure: func(b *deploymentViewRPC) {
				request := testDeploymentRequest(b.factory, testDeploymentRowInput(0))
				b.overrides[request] = deploymentRPCReply{data: testDeploymentWords(2, 38, 54, 17, 19)}
			},
			want: wantGenerationDeployment,
		},
		{
			name:      "marchid mismatch",
			configure: func(b *deploymentViewRPC) { b.march = 21 },
			want:      wantGenerationDeployment,
		},
		{
			name:      "pristine mismatch",
			configure: func(b *deploymentViewRPC) { b.pristine = common.HexToHash("0x123") },
			want:      wantGenerationDeployment,
		},
		{
			name:      "invalid geometry",
			configure: func(b *deploymentViewRPC) { b.rows[1][2] = 37 },
			want:      wantGenerationDeployment,
		},
		{
			name: "alternate two levels",
			configure: func(b *deploymentViewRPC) {
				b.rows = [][]uint64{{2, 44, 48, 17, 19, 23}, {2, 0, 44, 17, 19, 23}}
			},
			want: wantTableDeployment,
		},
		{
			name: "alternate three levels",
			configure: func(b *deploymentViewRPC) {
				b.rows = [][]uint64{{3, 44, 48, 17, 19, 23}, {3, 27, 17, 17, 19, 23}, {3, 0, 27, 17, 19, 23}}
				b.count = 3
			},
			want: wantTableDeployment,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, b := newDeploymentViewRPC(t, [][]uint64{{2, 38, 54, 17, 19, 23}, {2, 0, 38, 17, 19, 23}}, 2)
			tc.configure(b)
			firstHeader := &types.Header{Number: big.NewInt(100)}
			secondHeader := &types.Header{Number: big.NewInt(101)}
			b.head = txsender.BlockRef{Number: 100, Hash: firstHeader.Hash()}
			client := &ethClientMock{}
			client.On("HeaderByNumber", mock.Anything, big.NewInt(100)).Return(firstHeader, nil).Once()
			service := newDeploymentObserverService()
			service.client = client
			service.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
			app := prtRevertTestApp()
			app.IConsensusAddress = b.consensus
			key := deploymentIdentity(app)

			first, err := service.observeDeployment(t.Context(), app, adapter, b.head)
			require.NoError(t, err)
			require.Equal(t, wantPending, string(first.Assessment.State))
			require.Equal(t, tc.want, string(first.Assessment.PendingEvidence.Candidate))
			require.Equal(t, wantPending, string(service.deployments.capture(key).snapshot.Assessment.State))

			repeated, err := service.observeDeployment(t.Context(), app, adapter, b.head)
			require.NoError(t, err)
			require.Equal(t, wantPending, string(repeated.Assessment.State))
			require.Equal(t, wantPending, string(service.deployments.capture(key).snapshot.Assessment.State))

			b.head = txsender.BlockRef{Number: 101, Hash: secondHeader.Hash()}
			actual, err := service.observeDeployment(t.Context(), app, adapter, b.head)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(actual.Assessment.State), "the actual tier must become confirmed")
			published := service.deployments.capture(key).snapshot
			require.Equal(t, tc.want, string(published.Assessment.State))
			require.Equal(t, b.head, actual.Head)
			require.Equal(t, b.head, published.Head)
			require.False(t, service.publicationStopped())
			client.AssertExpectations(t)
		})
	}
}

func TestCompatibleDeploymentReadFailureDoesNotReportProtectionStop(t *testing.T) {
	f := newDeliveryIntervalFixture(t)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	f.checkpoint.epochs(f.epoch)
	f.block = f.facts
	f.snapshot(resultTestSnapshot(f.epoch, false))
	expectProtectionReads(t, f.factory, f.consensus, f.facts, f.epoch)
	cause := errors.New("deployment read unavailable")
	f.consensus.deploymentRead = func(_ context.Context, h txsender.BlockRef) (DeploymentEvidence, error) {
		return DeploymentEvidence{Head: h, Consensus: f.app.IConsensusAddress}, cause
	}
	require.ErrorIs(t, f.d.observeApplication(t.Context(), f.app, f.facts), cause)
	state, _, err := f.d.deploymentVerdictState(t.Context(), f.app)
	require.NoError(t, err)
	require.Equal(t, deploymentCompatible, state)
	require.NotContains(t, logs.String(), "PRT protection stopped")
}

func TestDeploymentReadShutdownDoesNotReportProtectionStop(t *testing.T) {
	f := newDeliveryIntervalFixture(t)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	f.checkpoint.epochs(f.epoch)
	expectProtectionReads(t, f.factory, f.consensus, f.facts, f.epoch)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f.consensus.deploymentRead = func(_ context.Context, h txsender.BlockRef) (DeploymentEvidence, error) {
		cancel()
		return DeploymentEvidence{Head: h, Consensus: f.app.IConsensusAddress}, context.Canceled
	}
	require.ErrorIs(t, f.d.observeApplication(ctx, f.app, f.facts), context.Canceled)
	require.NotContains(t, logs.String(), "PRT protection stopped")
}

func TestDeploymentResetAcknowledgementFailureReportsEffectiveAdmission(t *testing.T) {
	f := newDeliveryIntervalFixture(t)
	var logs bytes.Buffer
	f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	f.checkpoint.epochs(f.epoch)
	f.block = f.facts
	f.snapshot(resultTestSnapshot(f.epoch, false))
	expectProtectionReads(t, f.factory, f.consensus, f.facts, f.epoch)
	row := f.repo.deploymentRows[f.app.ID]
	row.ResetGeneration++
	f.repo.deploymentRows[f.app.ID] = row
	cause := errors.New("episode read unavailable")
	f.repo.deploymentRead = func(context.Context, repository.DeploymentIdentity) (*model.DeploymentAdmissionState, error) {
		return nil, cause
	}
	require.ErrorIs(t, f.d.observeApplication(t.Context(), f.app, f.facts), cause)
	require.Equal(t, deploymentCompatible, f.d.deployments.capture(deploymentIdentity(f.app)).snapshot.Assessment.State)
	f.repo.deploymentRead = nil
	effective, _, err := f.d.deploymentVerdictState(t.Context(), f.app)
	require.NoError(t, err)
	require.Equal(t, deploymentNone, effective)
	require.Contains(t, logs.String(), "PRT protection stopped")
	require.Contains(t, logs.String(), "join_close_block=310")
}

func TestUnknownDeploymentShutdownReportsOnlyIndependentFailure(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "shutdown only", true: "independent failure"}[mixed], func(t *testing.T) {
			f := newDeliveryIntervalFixture(t)
			setTestDeployment(f.d.Service, f.repo.prtRepositoryMock, f.app, 1, deploymentNone, nil)
			var logs bytes.Buffer
			f.d.Logger = slog.New(slog.NewTextHandler(&logs, nil))
			f.checkpoint.epochs(f.epoch)
			expectProtectionReads(t, f.factory, f.consensus, f.facts, f.epoch)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			dependency := errors.New("independent deployment failure")
			f.consensus.deploymentRead = func(_ context.Context, h txsender.BlockRef) (DeploymentEvidence, error) {
				cancel()
				cause := error(context.Canceled)
				if mixed {
					cause = errors.Join(cause, dependency)
				}
				return DeploymentEvidence{Head: h, Consensus: f.app.IConsensusAddress}, cause
			}
			err := f.d.observeApplication(ctx, f.app, f.facts)
			require.ErrorIs(t, err, context.Canceled)
			if mixed {
				require.ErrorIs(t, err, dependency)
				require.Contains(t, logs.String(), "PRT protection stopped")
			} else {
				require.NotContains(t, logs.String(), "PRT protection stopped")
			}
		})
	}
}

func TestDeploymentAssessmentRetainsMalformedIdentityAndDecodedZeroFactory(t *testing.T) {
	adapter, b := newDeploymentViewRPC(t, canonicalDeploymentRows(), 2)
	first, err := adapter.ReadDeployment(t.Context(), b.head)
	require.NoError(t, err)
	previous := planDeploymentAssessment(deploymentSnapshot{}, first, nil, true)
	b.head = deploymentTestHead(101)
	b.absent[common.Address{}] = true
	factoryRequest := testDeploymentRequest(b.consensus, testDeploymentSelector(testFactorySignature))
	b.overrides[factoryRequest] = deploymentRPCReply{data: testDeploymentAddress(common.Address{})}
	zero, err := adapter.ReadDeployment(t.Context(), b.head)
	require.NoError(t, err)
	require.True(t, zero.FactoryKnown)
	require.False(t, planDeploymentAssessment(previous, zero, nil, true).Alpha6Shape)
	request := testDeploymentRequest(b.factory, testDeploymentRowInput(0))
	delete(b.overrides, testDeploymentRequest(b.consensus, testDeploymentSelector(testFactorySignature)))
	b.overrides[request] = deploymentRPCReply{data: []byte{1}}
	malformed, err := adapter.ReadDeployment(t.Context(), b.head)
	require.NoError(t, err)
	pending := planDeploymentAssessment(deploymentSnapshot{}, malformed, nil, true)
	b.head = deploymentTestHead(102)
	b.overrides[request] = deploymentRPCReply{data: []byte{2, 3}}
	changed, err := adapter.ReadDeployment(t.Context(), b.head)
	require.NoError(t, err)
	require.Equal(t, deploymentPending, planDeploymentAssessment(pending, changed, nil, true).Assessment.State)
	b.overrides[request] = deploymentRPCReply{data: []byte{2}}
	sameLength, err := adapter.ReadDeployment(t.Context(), b.head)
	require.NoError(t, err)
	require.Equal(t, deploymentPending, planDeploymentAssessment(pending, sameLength, nil, true).Assessment.State)

}

func TestCompatibleDeploymentCacheUsesOnlyCurrentPinnedHeadAndEpisode(t *testing.T) {
	s := newDeploymentObserverService()
	app := prtRevertTestApp()
	reads := 0
	reader := &daveConsensusAdapterMock{deploymentRead: func(_ context.Context, h txsender.BlockRef) (DeploymentEvidence, error) {
		reads++
		evidence := compatibleDeploymentEvidence(h)
		if reads == 2 {
			evidence.Candidate, evidence.Cause = deploymentGeneration, deploymentCauseMarchID
			evidence.MarchID--
		}
		return evidence, nil
	}}
	head := deploymentTestHead(100)
	_, err := s.observeDeployment(t.Context(), app, reader, head)
	require.NoError(t, err)
	_, err = s.observeDeployment(t.Context(), app, reader, head)
	require.NoError(t, err)
	require.Equal(t, 1, reads)
	repo := s.repository.(*prtRepositoryMock)
	row := repo.deploymentRows[app.ID]
	row.ResetGeneration++
	repo.deploymentRows[app.ID] = row
	snapshot, err := s.observeDeployment(t.Context(), app, reader, head)
	require.NoError(t, err)
	require.Equal(t, 2, reads, "a same-head reset must read new evidence before using the compatible cache")
	require.Equal(t, deploymentPending, snapshot.Assessment.State)
	_, err = s.observeDeployment(t.Context(), app, reader, deploymentTestHead(101))
	require.NoError(t, err)
	require.Equal(t, 3, reads)
}

func TestDeploymentSameHeadRetriesNoneAndPending(t *testing.T) {
	for _, candidate := range []deploymentState{deploymentNone, deploymentGeneration} {
		t.Run(string(candidate), func(t *testing.T) {
			s := newDeploymentObserverService()
			app := prtRevertTestApp()
			reads := 0
			reader := &daveConsensusAdapterMock{deploymentRead: func(_ context.Context, h txsender.BlockRef) (DeploymentEvidence, error) {
				reads++
				return DeploymentEvidence{Head: h, Consensus: app.IConsensusAddress,
					Candidate: candidate, Cause: deploymentCauseStaticShape}, nil
			}}
			head := deploymentTestHead(100)
			_, err := s.observeDeployment(t.Context(), app, reader, head)
			require.NoError(t, err)
			_, err = s.observeDeployment(t.Context(), app, reader, head)
			require.NoError(t, err)
			require.Equal(t, 2, reads)
		})
	}
}
