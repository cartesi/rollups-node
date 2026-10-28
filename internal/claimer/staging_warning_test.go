// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const stagingContradictionWarning = "Claim publication waiting: recorded staging contradicts latest view"

func TestStagingContradictionWarnsAcrossPublicationPaths(t *testing.T) {
	for _, entry := range []string{claimFreshEntry, claimRetainedEntry, claimRecheckEntry} {
		t.Run(entry, func(t *testing.T) {
			s, backend, lookup := newDeliveryFixture(t)
			app, epoch := queueDelivery(t, backend, lookup, 1)
			epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
			backend.claims[app.ID] = makeClaimStatus(claimStatusStaged, epoch, 30)
			switch entry {
			case claimFreshEntry:
				delete(backend.claims, app.ID)
			case claimRetainedEntry:
				require.NoError(t, s.publisher.tick(t.Context()))
				require.Len(t, backend.published, 1)
				delete(backend.claims, app.ID)
			case claimRecheckEntry:
				useClaimSigningHook(t, s, backend, func() { delete(backend.claims, app.ID) })
			}
			logs := capturePublicationLogs(s)
			before := len(backend.published)
			for range 3 {
				require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrInconclusive)
			}
			require.Len(t, backend.published, before, "the warning cannot authorize publication or cancellation")
			require.Equal(t, 1, strings.Count(logs.String(), stagingContradictionWarning))
			for _, field := range []string{"level=WARN", "application=" + app.Name, "epoch=3",
				"staged_at_block=30", "latest_block=50", "observation_policy=FINALIZED"} {
				require.Contains(t, logs.String(), field)
			}
			key := claimAction{ApplicationID: app.ID, Address: app.IApplicationAddress, Epoch: epoch.Index}
			s.publisher.stagingWarnings[key] = time.Now().Add(-time.Second)
			require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrInconclusive)
			require.Equal(t, 2, strings.Count(logs.String(), stagingContradictionWarning))
		})
	}
}

func TestStagingWarningKeysDoNotShareOtherRefusalClocks(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	logs := capturePublicationLogs(s)
	app, epoch := queueDelivery(t, backend, lookup, 1)
	epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
	work := repository.ClaimWork{Application: app, Epoch: epoch}
	s.publisher.cooldowns[claimPlanningRefusal(work)] = time.Now().Add(time.Hour)
	require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrInconclusive)
	require.Equal(t, 1, strings.Count(logs.String(), stagingContradictionWarning), "other refusals must not hide this cause")

	block := txsender.BlockRef{Number: backend.last, Hash: deliveryHeader(backend.last).Hash()}
	later := makeComputedEpoch(app, epoch.Index+1)
	later.Status, later.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
	lookup.add(app, later)
	_, _, err := s.publisher.decide(t.Context(), block, repository.ClaimWork{Application: app, Epoch: later}, false)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	other, otherEpoch := queueDelivery(t, backend, lookup, 2)
	otherEpoch.Status, otherEpoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
	_, _, err = s.publisher.decide(t.Context(), block, repository.ClaimWork{Application: other, Epoch: otherEpoch}, false)
	require.ErrorIs(t, err, txsender.ErrInconclusive)
	require.Equal(t, 3, strings.Count(logs.String(), stagingContradictionWarning),
		"different apps and epochs have their own diagnostic clocks")
}

func TestStagingWarningDoesNotDelayRecoveredPublication(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, epoch := queueDelivery(t, backend, lookup, 1)
	epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
	require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrInconclusive)
	key := claimAction{ApplicationID: app.ID, Address: app.IApplicationAddress, Epoch: epoch.Index}
	require.True(t, time.Now().Before(s.publisher.stagingWarnings[key]))
	backend.claims[app.ID] = makeClaimStatus(claimStatusStaged, epoch, 30)
	require.NoError(t, s.publisher.tick(t.Context()))
	require.Len(t, backend.published, 1, "the diagnostic interval is not an action cooldown")
	require.Equal(t, "acceptClaim", publishedClaimMethod(t, backend.published[0].Data()))
}

func TestStagingWarningPrecedesObserverMaturityWithoutDiagnosing(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	app, epoch := queueDelivery(t, backend, lookup, 1)
	app.ClaimStagingPeriod = 100
	epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
	logs := capturePublicationLogs(s)
	require.ErrorIs(t, s.publisher.tick(t.Context()), txsender.ErrInconclusive)
	require.Contains(t, logs.String(), stagingContradictionWarning)
	chain := s.blockchain.(*claimerBlockchainMock)
	chain.On("getConsensusAddress", mock.Anything, app, big.NewInt(45)).Return(app.IConsensusAddress, nil).Once()
	result := s.processStagedClaim(t.Context(), claimWork{app: app, epoch: epoch}, big.NewInt(45))
	require.NoError(t, result.err)
	require.Zero(t, result.progress)
	require.Empty(t, s.repository.(*claimerRepositoryMock).Calls, "no status write before the observer's maturity check")
	chain.AssertExpectations(t) // An unexpected getClaim call would fail the mock.
}

func TestStagingWarningRequiresTheSpecificContradiction(t *testing.T) {
	const (
		latestCase      = "latest"
		safeCase        = "safe"
		submittedCase   = "submitted"
		rpcFailureCase  = "RPC error"
		laggingHeadCase = "lagging head"
	)
	for _, scenario := range []string{latestCase, safeCase, submittedCase, laggingHeadCase, rpcFailureCase} {
		t.Run(scenario, func(t *testing.T) {
			policy := model.DefaultBlock_Finalized
			if scenario == latestCase {
				policy = model.DefaultBlock_Latest
			}
			if scenario == safeCase {
				policy = model.DefaultBlock_Safe
			}
			s, backend, lookup := newDeliveryFixture(t, policy)
			_, epoch := queueDelivery(t, backend, lookup, 1)
			epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(uint64(30))
			switch scenario {
			case submittedCase:
				epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimSubmitted, nil
			case laggingHeadCase:
				epoch.StagedAtBlock = new(backend.last + 1)
			case rpcFailureCase:
				backend.claimReadErr = errors.New("claim RPC unavailable")
			}
			logs := capturePublicationLogs(s)
			err := s.publisher.tick(t.Context())
			if scenario == laggingHeadCase || scenario == rpcFailureCase {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NotContains(t, logs.String(), stagingContradictionWarning)
		})
	}
}
