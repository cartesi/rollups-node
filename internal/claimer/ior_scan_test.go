// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/txsender"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func (m *claimerBlockchainMock) claimReference(ctx context.Context, number uint64) (txsender.BlockRef, error) {
	args := m.Called(ctx, number)
	return args.Get(0).(txsender.BlockRef), args.Error(1)
}

func (m *claimerBlockchainMock) checkClaimReference(ctx context.Context, ref txsender.BlockRef) error {
	return m.Called(ctx, ref).Error(0)
}

func (*eventChain) claimReference(ctx context.Context, number uint64) (txsender.BlockRef, error) {
	return txsender.BlockRef{Number: number, Hash: deliveryHeader(number).Hash()}, ctx.Err()
}

func (*eventChain) checkClaimReference(ctx context.Context, _ txsender.BlockRef) error {
	return ctx.Err()
}

const (
	iorScanTestHead    uint64 = 100
	iorScanTestStage   uint64 = 80
	iorAcceptedStream         = "accepted"
	iorSubmittedStream        = "submitted"
	iorStagedStream           = "staged"
)

type iorScanWindow struct {
	stream   string
	from, to uint64
}

type iorScanTestChain struct {
	t *testing.T
	*eventChain
	orphanedTransaction         bool
	branchChanged               bool
	ignoreReferenceCancellation bool
	head                        uint64
	pointStatus                 uint8
	pointErr                    error
	pointBlock                  uint64
	accepted                    *iconsensus.IConsensusClaimAccepted
	submitted                   []*iconsensus.IConsensusClaimSubmitted
	staged                      *iconsensus.IConsensusClaimStaged
	scanErrors                  map[string]error
	referenceError              func(txsender.BlockRef) error
	onScan                      func(string)
	order                       []string
	windows                     []iorScanWindow
}

func (c *iorScanTestChain) getDefaultBlockNumber(context.Context) (*big.Int, error) {
	return new(big.Int).SetUint64(c.head), nil
}

func (c *iorScanTestChain) getClaimStatus(
	ctx context.Context, _ *model.Application, epoch *model.Epoch, block *big.Int,
) (iconsensus.IConsensusClaim, error) {
	require.Equal(c.t, new(big.Int).SetUint64(c.head), block, "the point read must use the selected observation height")
	c.order = append(c.order, "point")
	return makeClaimStatus(c.pointStatus, epoch, c.pointBlock), errors.Join(c.pointErr, ctx.Err())
}

func (c *iorScanTestChain) checkClaimReference(ctx context.Context, ref txsender.BlockRef) error {
	if c.referenceError != nil {
		if err := c.referenceError(ref); err != nil {
			return err
		}
	}
	refNow, err := c.claimReference(ctx, ref.Number)
	if err != nil && !c.ignoreReferenceCancellation {
		return err
	}
	if refNow.Hash != ref.Hash {
		return txsender.ErrNonCanonical
	}
	if c.ignoreReferenceCancellation {
		return nil
	}
	return ctx.Err()
}

func (c *iorScanTestChain) scan(ctx context.Context, stream string, from, to uint64) error {
	c.order = append(c.order, stream)
	c.windows = append(c.windows, iorScanWindow{stream, from, to})
	if c.onScan != nil {
		c.onScan(stream)
	}
	return errors.Join(c.scanErrors[stream], ctx.Err())
}

func (c *iorScanTestChain) findClaimAcceptedEventAndSucc(
	ctx context.Context, _ *model.Application, _ *model.Epoch, from, to uint64,
) (*iconsensus.IConsensus, *iconsensus.IConsensusClaimAccepted, *iconsensus.IConsensusClaimAccepted, error) {
	if err := c.scan(ctx, iorAcceptedStream, from, to); err != nil {
		return nil, nil, nil, err
	}
	if c.accepted != nil && c.accepted.Raw.BlockNumber >= from && c.accepted.Raw.BlockNumber <= to {
		return nil, c.accepted, nil, nil
	}
	return nil, nil, nil, nil
}

func (c *iorScanTestChain) findClaimSubmittedEventAndSucc(
	ctx context.Context, _ *model.Application, _ *model.Epoch, from, to uint64,
) (*iconsensus.IConsensus, []*iconsensus.IConsensusClaimSubmitted, error) {
	if err := c.scan(ctx, iorSubmittedStream, from, to); err != nil {
		return nil, nil, err
	}
	var events []*iconsensus.IConsensusClaimSubmitted
	for _, event := range c.submitted {
		if event.Raw.BlockNumber >= from && event.Raw.BlockNumber <= to {
			events = append(events, event)
		}
	}
	return nil, events, nil
}

func (c *iorScanTestChain) findClaimStagedEventAndSucc(
	ctx context.Context, _ *model.Application, _ *model.Epoch, from, to uint64,
) (*iconsensus.IConsensus, *iconsensus.IConsensusClaimStaged, *iconsensus.IConsensusClaimStaged, error) {
	if err := c.scan(ctx, iorStagedStream, from, to); err != nil {
		return nil, nil, nil, err
	}
	if c.staged != nil && c.staged.Raw.BlockNumber >= from && c.staged.Raw.BlockNumber <= to {
		return nil, c.staged, nil, nil
	}
	return nil, nil, nil, nil
}

func newIORScanFixture(t *testing.T, phase int) (
	*Service, *claimerRepositoryMock, *iorScanTestChain, *model.Application, *model.Epoch, *bytes.Buffer,
) {
	t.Helper()
	s, repo, _ := newServiceMock(t)
	chain := &iorScanTestChain{
		t: t, eventChain: &eventChain{}, head: iorScanTestHead, pointBlock: iorScanTestStage, scanErrors: map[string]error{},
	}
	s.blockchain, s.defaultBlock = chain, model.DefaultBlock_Finalized
	logs := new(bytes.Buffer)
	s.Logger = slog.New(slog.NewJSONHandler(logs, nil))
	app := makeApplication()
	app.ID = 1
	epoch := makeComputedEpoch(app, 3)
	switch phase {
	case claimStagePage:
		epoch.Status = model.EpochStatus_ClaimSubmitted
	case claimAcceptPage:
		epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(iorScanTestStage)
		chain.pointStatus = claimStatusStaged
	}
	epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
	return s, repo, chain, app, epoch, logs
}

func runIORScan(ctx context.Context, s *Service, phase int, app *model.Application, epoch *model.Epoch, head uint64) claimStepResult {
	block := new(big.Int).SetUint64(head)
	work := claimWork{app: app, epoch: epoch}
	switch phase {
	case claimStagePage:
		return s.processSubmittedClaim(ctx, work, block)
	case claimAcceptPage:
		return s.processAcceptedClaimEvent(ctx, work, block)
	default:
		return s.processComputedClaim(ctx, work, block)
	}
}

func TestIORFinalizedScanRetainsCompletedPrefixAndReadsExactSuffix(t *testing.T) {
	for _, phase := range []int{claimSubmitPage, claimStagePage} {
		t.Run(map[int]string{claimSubmitPage: computedVisibilityPath, claimStagePage: iorSubmittedStream}[phase], func(t *testing.T) {
			s, repo, chain, app, epoch, _ := newIORScanFixture(t, phase)
			first := runIORScan(t.Context(), s, phase, app, epoch, chain.head)
			require.NoError(t, first.err)
			require.Zero(t, first.progress)
			initial := append([]iorScanWindow(nil), chain.windows...)
			require.NotEmpty(t, initial)
			require.Equal(t, "point", chain.order[0])
			for _, window := range initial {
				require.Equal(t, epoch.LastBlock+1, window.from)
				require.Equal(t, chain.head, window.to)
			}
			again := runIORScan(t.Context(), s, phase, app, epoch, chain.head)
			require.NoError(t, again.err)
			require.Zero(t, again.progress, "memory progress must not cause a busy reschedule loop")
			require.Equal(t, initial, chain.windows, "validated finalized history must not be fetched twice")
			chain.head++
			next := runIORScan(t.Context(), s, phase, app, epoch, chain.head)
			require.NoError(t, next.err)
			require.Len(t, chain.windows, len(initial)*2)
			for _, window := range chain.windows[len(initial):] {
				require.Equal(t, chain.head, window.from, "the next unread block must be included")
				require.Equal(t, chain.head, window.to)
			}
			repo.AssertNotCalled(t, "UpdateEpochWithAcceptedClaim", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestIORScanDetectsAcceptanceAtFirstUnreadBlockAndKeepsEventHash(t *testing.T) {
	s, repo, chain, app, epoch, logs := newIORScanFixture(t, claimSubmitPage)
	require.NoError(t, runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head).err)
	chain.head++
	chain.pointStatus = claimStatusAccepted
	event := makeAcceptedEvent(app, epoch)
	event.Raw.BlockNumber, event.Raw.TxHash = chain.head, common.HexToHash("0xfeed")
	chain.accepted = event
	repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, new(event.Raw.TxHash)).Return(nil).Once()
	result := runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head)
	require.NoError(t, result.err)
	require.Equal(t, 1, result.progress)
	require.Equal(t, iorScanWindow{iorAcceptedStream, chain.head, chain.head}, chain.windows[len(chain.windows)-1])
	require.Empty(t, s.iorScans)
	records := claimerIORRecords(t, logs, claimerIORAccepted)
	require.Len(t, records, 1)
	require.Equal(t, "claim_accepted_event", records[0]["observation_source"])
	require.Equal(t, event.Raw.TxHash.Hex(), records[0]["event_transaction"])
	repo.AssertExpectations(t)
}

func TestIORPointFailureAndUnstagedStatePreserveForeignAcceptance(t *testing.T) {
	for _, pointFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "unstaged local key", true: "point RPC fails"}[pointFails], func(t *testing.T) {
			s, repo, chain, app, epoch, _ := newIORScanFixture(t, claimSubmitPage)
			app.ConsensusType = model.Consensus_Quorum
			if pointFails {
				chain.pointErr = errors.New("point endpoint unavailable")
			}
			event := makeAcceptedEvent(app, epoch)
			event.MachineMerkleRoot[0] ^= 1
			event.Raw.BlockNumber = chain.head
			chain.accepted = event
			repo.On("RejectEpochAndSetApplicationDiverged", mock.Anything, app.ID, epoch.Index, mock.Anything).
				Return(repository.RejectEpochAndDivergeResult{EpochRejected: true, ApplicationDiverged: true}, nil).Once()
			result := runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head)
			require.Error(t, result.err)
			if pointFails {
				require.ErrorIs(t, result.err, chain.pointErr)
			}
			require.Equal(t, model.ApplicationStatus_Diverged, app.Status)
			require.Equal(t, model.EpochStatus_ClaimRejected, epoch.Status)
			require.Empty(t, s.iorScans)
			repo.AssertExpectations(t)
		})
	}
}

func TestIORScanFailureStillProjectsPointEvidence(t *testing.T) {
	for _, phase := range []int{claimSubmitPage, claimStagePage} {
		for _, status := range []uint8{claimStatusStaged, claimStatusAccepted} {
			t.Run(map[int]string{claimSubmitPage: computedVisibilityPath, claimStagePage: iorSubmittedStream}[phase]+"/"+
				map[uint8]string{claimStatusStaged: iorStagedStream, claimStatusAccepted: iorAcceptedStream}[status], func(t *testing.T) {
				s, repo, chain, app, epoch, logs := newIORScanFixture(t, phase)
				scanErr := errors.New("historical counter rate limited")
				stream := iorAcceptedStream
				if phase == claimStagePage {
					stream = iorStagedStream
				}
				chain.scanErrors[stream], chain.pointStatus = scanErr, status
				condition := claimerIORStage
				switch {
				case status == claimStatusAccepted:
					condition = claimerIORAccepted
					repo.On("UpdateEpochWithAcceptedClaim", mock.Anything, app.ID, epoch.Index, (*common.Hash)(nil)).
						Return(nil).Once()
				case phase == claimSubmitPage:
					repo.On("UpdateEpochReconciledStaged", mock.Anything, app.ID, epoch.Index, chain.pointBlock).Return(nil).Once()
				default:
					repo.On("UpdateEpochToStaged", mock.Anything, app.ID, epoch.Index, chain.pointBlock).Return(nil).Once()
				}
				result := runIORScan(t.Context(), s, phase, app, epoch, chain.head)
				require.ErrorIs(t, result.err, scanErr)
				require.Equal(t, 1, result.progress)
				require.Equal(t, []string{"point", stream}, chain.order)
				require.Len(t, claimerIORRecords(t, logs, condition), 1)
				require.Empty(t, s.iorScans)
				require.Equal(t, model.ApplicationStatus_OK, app.Status)
				repo.AssertExpectations(t)
			})
		}
	}
}

func TestIORFailedProjectionDoesNotReportAndRetryUsesOriginalRange(t *testing.T) {
	s, repo, chain, app, epoch, logs := newIORScanFixture(t, claimSubmitPage)
	scanErr, writeErr := errors.New("scan incomplete"), errors.New("write failed")
	chain.scanErrors[iorAcceptedStream], chain.pointStatus = scanErr, claimStatusStaged
	repo.On("UpdateEpochReconciledStaged", mock.Anything, app.ID, epoch.Index, chain.pointBlock).Return(writeErr).Once()
	result := runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head)
	require.ErrorIs(t, result.err, scanErr)
	require.ErrorIs(t, result.err, writeErr)
	require.Zero(t, result.progress)
	require.Empty(t, claimerIORRecords(t, logs, claimerIORStage))
	require.Empty(t, s.iorScans)
	repo.On("UpdateEpochReconciledStaged", mock.Anything, app.ID, epoch.Index, chain.pointBlock).Return(nil).Once()
	retry := runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head)
	require.ErrorIs(t, retry.err, scanErr)
	require.Equal(t, 1, retry.progress)
	require.Equal(t, chain.windows[0], chain.windows[1])
	require.Len(t, claimerIORRecords(t, logs, claimerIORStage), 1)
	repo.AssertExpectations(t)
}

func TestIORSharedPrefixWaitsForBothStreamsAndCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "submitted scan fails", true: "cancel during accepted scan"}[cancelled], func(t *testing.T) {
			s, _, chain, app, epoch, _ := newIORScanFixture(t, claimSubmitPage)
			require.NoError(t, runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head).err)
			chain.head++
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if cancelled {
				chain.onScan = func(string) { cancel() }
			} else {
				chain.scanErrors[iorSubmittedStream] = errors.New("submitted logs incomplete")
			}
			result := runIORScan(ctx, s, claimSubmitPage, app, epoch, chain.head)
			require.Error(t, result.err)
			if cancelled {
				require.ErrorIs(t, result.err, context.Canceled)
			}
			key := iorScanKey{app.ID, epoch.Index, claimSubmitPage}
			require.Equal(t, iorScanTestHead, s.iorScans[key].through.Number)
			chain.scanErrors, chain.onScan = map[string]error{}, nil
			before := len(chain.windows)
			require.NoError(t, runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head).err)
			require.Len(t, chain.windows, before+2)
			for _, window := range chain.windows[before:] {
				require.Equal(t, iorScanTestHead+1, window.from, "failed range must remain unread")
			}
			require.Equal(t, chain.head, s.iorScans[key].through.Number)
		})
	}
}

func TestIORProvisionalObservationsAlwaysReplayFullHistory(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Safe, model.DefaultBlock_Latest} {
		for _, phase := range []int{claimSubmitPage, claimStagePage, claimAcceptPage} {
			name := policy.String() + "/" + map[int]string{
				claimSubmitPage: computedVisibilityPath, claimStagePage: iorSubmittedStream, claimAcceptPage: iorStagedStream,
			}[phase]
			t.Run(name, func(t *testing.T) {
				s, _, chain, app, epoch, _ := newIORScanFixture(t, phase)
				s.defaultBlock = policy
				require.NoError(t, runIORScan(t.Context(), s, phase, app, epoch, chain.head).err)
				initial := append([]iorScanWindow(nil), chain.windows...)
				require.NotEmpty(t, initial)
				require.Empty(t, s.iorScans)
				for _, step := range []string{"same head", "new head", "branch change", "head regression"} {
					switch step {
					case "new head":
						chain.head++
					case "branch change":
						chain.branchChanged = true
						chain.head++
					case "head regression":
						chain.head--
					}
					before := len(chain.windows)
					chain.order = nil
					result := runIORScan(t.Context(), s, phase, app, epoch, chain.head)
					require.NoError(t, result.err, step)
					require.Zero(t, result.progress, step)
					require.Equal(t, "point", chain.order[0], step)
					require.Len(t, chain.windows, before+len(initial), step)
					for _, window := range chain.windows[before:] {
						require.Equal(t, epoch.LastBlock+1, window.from, step)
						require.Equal(t, chain.head, window.to, step)
					}
					require.Empty(t, s.iorScans, "provisional visits never claim a cached prefix")
				}
			})
		}
	}
}

func TestIORFinalizedReferenceContradictionStopsProjection(t *testing.T) {
	s, repo, chain, app, epoch, logs := newIORScanFixture(t, claimSubmitPage)
	require.NoError(t, runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head).err)
	before := len(chain.windows)
	chain.referenceError = func(txsender.BlockRef) error { return txsender.ErrNonCanonical }
	chain.pointStatus = claimStatusStaged
	result := runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head)
	require.ErrorIs(t, result.err, txsender.ErrFinality)
	require.Equal(t, before, len(chain.windows))
	require.Empty(t, claimerIORRecords(t, logs, claimerIORStage))
	require.Empty(t, repo.Calls)
}

func TestIORFailedEndReferenceDoesNotAcknowledgeHistory(t *testing.T) {
	s, _, chain, app, epoch, _ := newIORScanFixture(t, claimSubmitPage)
	chain.referenceError = func(txsender.BlockRef) error { return txsender.ErrInconclusive }
	result := runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head)
	require.ErrorIs(t, result.err, txsender.ErrInconclusive)
	require.Empty(t, s.iorScans)
	before := len(chain.windows)
	chain.referenceError = nil
	require.NoError(t, runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head).err)
	require.Len(t, chain.windows, before*2)
	for _, window := range chain.windows[before:] {
		require.Equal(t, epoch.LastBlock+1, window.from)
	}
}

func TestIORScanLifetimePreservesPagedTailsAndStageMarkers(t *testing.T) {
	s, repo, _, app, epoch, _ := newIORScanFixture(t, claimSubmitPage)
	tail := iorScanKey{app.ID, epoch.Index + 1, claimSubmitPage}
	otherApp := iorScanKey{app.ID + 1, epoch.Index, claimSubmitPage}
	submitted := iorScanKey{app.ID, epoch.Index, claimStagePage}
	first := iorScanKey{app.ID, epoch.Index, claimSubmitPage}
	s.iorScans = map[iorScanKey]iorScanProgress{
		first: {seen: true}, tail: {seen: true}, otherApp: {seen: true}, submitted: {seen: true},
	}
	marker := iorStageKey{app.ID, epoch.Index}
	s.iorStages = map[iorStageKey]uint64{marker: iorScanTestStage}
	// A nonzero empty page can be a deletion/disable race, not an empty source.
	s.repository = &iorEmptyPageRepository{claimerRepositoryMock: repo, applicationID: app.ID}
	_, err := s.selectClaimPage(t.Context(), claimSubmitPage)
	require.NoError(t, err)
	_, err = s.selectClaimPage(t.Context(), claimSubmitPage)
	require.NoError(t, err)
	require.Contains(t, s.iorScans, tail)
	// Another complete app rotation without the other app releases its keys.
	s.visitIORScanApplication(claimSubmitPage, app.ID, app.ID)
	require.NotContains(t, s.iorScans, otherApp)
	require.Contains(t, s.iorScans, tail, "an epoch outside the page is still active app work")
	s.visitIORScanApplication(claimSubmitPage, app.ID, 0)
	require.Len(t, s.iorScans, 1)
	require.Contains(t, s.iorScans, submitted)
	require.Equal(t, iorScanTestStage, s.iorStages[marker], "empty computed work must not erase staged markers")
	repo.AssertExpectations(t)
}

type iorLifecycleRepository struct{ *eventRepository }

func (r *iorLifecycleRepository) UpdateEpochReconciledStaged(ctx context.Context, id int64, index, block uint64) error {
	epoch := r.epoch(id, index)
	if epoch.Status != model.EpochStatus_ClaimComputed {
		return repository.ErrNoUpdate
	}
	epoch.Status, epoch.StagedAtBlock = model.EpochStatus_ClaimStaged, new(block)
	return ctx.Err()
}

func TestIORReaderQuorumTickKeepsSubmittedScanAndObservesExternalStage(t *testing.T) {
	s, backend, lookup := newDeliveryFixture(t)
	repo := &iorLifecycleRepository{&eventRepository{deliveryCandidateRepository: &deliveryCandidateRepository{lookup: lookup}}}
	chain := &iorScanTestChain{
		t: t, eventChain: &eventChain{}, head: iorScanTestHead, pointBlock: iorScanTestHead + 1, scanErrors: map[string]error{},
	}
	s.repository, s.blockchain, s.defaultBlock, s.submissionEnabled = repo, chain, model.DefaultBlock_Finalized, false
	logs := new(bytes.Buffer)
	s.Logger = slog.New(slog.NewJSONHandler(logs, nil))
	app, epoch := deliveryApplication(1, model.Consensus_Quorum)
	epoch.CalculatedOutputsRoot = new(common.HexToHash("0xbad"))
	lookup.add(app, epoch)
	event := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash("0xfeed"))
	event.Raw.BlockNumber = chain.head
	chain.submitted = []*iconsensus.IConsensusClaimSubmitted{event}
	progressed, err := s.Tick(t.Context())
	require.NoError(t, err)
	require.True(t, progressed)
	require.Equal(t, model.EpochStatus_ClaimSubmitted, epoch.Status)
	require.Equal(t, event.Raw.TxHash, *epoch.ClaimTransactionHash)
	before := len(chain.windows)
	progressed, err = s.Tick(t.Context())
	require.NoError(t, err)
	require.False(t, progressed)
	require.Equal(t, before, len(chain.windows))
	chain.head++
	chain.pointStatus = claimStatusStaged
	chain.staged = makeStagedEvent(app, epoch)
	chain.staged.Raw.BlockNumber = chain.head
	progressed, err = s.Tick(t.Context())
	require.NoError(t, err)
	require.True(t, progressed)
	require.Equal(t, model.EpochStatus_ClaimStaged, epoch.Status)
	require.Equal(t, chain.head, *epoch.StagedAtBlock)
	require.Len(t, claimerIORRecords(t, logs, claimerIORStage), 1)
	key := iorScanKey{app.ID, epoch.Index, claimAcceptPage}
	require.Len(t, s.iorScans, 1)
	require.Equal(t, chain.head, s.iorScans[key].through.Number)
	before = len(chain.windows)
	chain.order = nil
	progressed, err = s.Tick(t.Context())
	require.NoError(t, err)
	require.False(t, progressed)
	require.Equal(t, []string{"point"}, chain.order, "the combined staged pass must read the point exactly once")
	require.Equal(t, before, len(chain.windows), "retained staged IOR must reuse its accepted-event prefix")
	require.Empty(t, backend.published)
	require.Equal(t, model.ApplicationStatus_OK, app.Status)
}

type iorEmptyPageRepository struct {
	*claimerRepositoryMock
	applicationID int64
}

func (r *iorEmptyPageRepository) SelectClaimsToSubmitPage(
	context.Context, repository.ClaimCursor, uint64,
) (repository.ClaimPage, error) {
	return repository.ClaimPage{ApplicationID: r.applicationID}, nil
}

func (*iorScanTestChain) claimSubmitterAddress() (common.Address, bool) {
	return common.HexToAddress("0x1"), true
}

func TestIORRestartReconstructsHistoryWithoutReusingMemory(t *testing.T) {
	first, _, chain, app, epoch, _ := newIORScanFixture(t, claimSubmitPage)
	require.NoError(t, runIORScan(t.Context(), first, claimSubmitPage, app, epoch, chain.head).err)
	require.NotEmpty(t, first.iorScans)
	second, _, _ := newServiceMock(t)
	second.blockchain, second.defaultBlock = chain, model.DefaultBlock_Finalized
	before := len(chain.windows)
	require.NoError(t, runIORScan(t.Context(), second, claimSubmitPage, app, epoch, chain.head).err)
	require.Len(t, chain.windows, before*2)
	for _, window := range chain.windows[before:] {
		require.Equal(t, epoch.LastBlock+1, window.from)
	}
}

func TestIORPointStageReportsBeforeProvisionalAuditRepair(t *testing.T) {
	for _, repairFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "repair succeeds", true: "repair fails"}[repairFails], func(t *testing.T) {
			s, repo, chain, app, epoch, logs := newIORScanFixture(t, claimStagePage)
			s.defaultBlock = model.DefaultBlock_Latest
			chain.orphanedTransaction = true
			chain.pointStatus = claimStatusStaged
			originalHash := *epoch.ClaimTransactionHash
			originalErr := errors.New("initial stage scan failed")
			chain.scanErrors[iorStagedStream] = originalErr
			calls := 0
			chain.onScan = func(stream string) {
				if stream != iorStagedStream {
					return
				}
				calls++
				if calls > 1 {
					require.Len(t, claimerIORRecords(t, logs, claimerIORStage), 1,
						"the verified projection must be reported before another fallible history read")
					if !repairFails {
						delete(chain.scanErrors, iorStagedStream)
					}
				}
			}
			chain.staged = makeStagedEvent(app, epoch)
			chain.staged.Raw.BlockNumber = chain.pointBlock
			chain.staged.Raw.TxHash = common.HexToHash("0xfeed")
			repo.On("UpdateEpochToStaged", mock.Anything, app.ID, epoch.Index, chain.pointBlock).Return(nil).Once()
			if !repairFails {
				repo.On("RefreshClaimObservation", mock.Anything,
					mock.MatchedBy(func(e *model.Epoch) bool {
						return e.Status == model.EpochStatus_ClaimStaged && e.StagedAtBlock != nil &&
							*e.StagedAtBlock == chain.pointBlock && *e.ClaimTransactionHash == originalHash
					}), new(chain.staged.Raw.TxHash), new(chain.pointBlock)).Return(nil).Once()
			}
			result := runIORScan(t.Context(), s, claimStagePage, app, epoch, chain.head)
			require.ErrorIs(t, result.err, originalErr)
			require.Equal(t, 1, result.progress)
			require.Len(t, claimerIORRecords(t, logs, claimerIORStage), 1)
			require.Equal(t, originalHash, *epoch.ClaimTransactionHash, "a repair uses a copy of the committed staging row")
			require.Empty(t, s.iorScans)
			repo.AssertExpectations(t)
		})
	}
}

func TestIORPointStageNeverAttributesPeerVoteToSubmittingQuorum(t *testing.T) {
	s, repo, chain, app, epoch, logs := newIORScanFixture(t, claimStagePage)
	s.defaultBlock = model.DefaultBlock_Safe
	app.ConsensusType = model.Consensus_Quorum
	epoch.ClaimTransactionHash = nil
	chain.pointStatus = claimStatusStaged
	scanErr := errors.New("stage logs initially unavailable")
	chain.scanErrors[iorStagedStream] = scanErr
	calls := 0
	chain.onScan = func(stream string) {
		if stream == iorStagedStream {
			calls++
			if calls > 1 {
				delete(chain.scanErrors, stream)
				require.Len(t, claimerIORRecords(t, logs, claimerIORStage), 1)
			}
		}
	}
	chain.staged = makeStagedEvent(app, epoch)
	chain.staged.Raw.BlockNumber = chain.pointBlock
	peer := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash("0xfeed"))
	peer.Submitter, peer.Raw.BlockNumber = common.HexToAddress("0x2"), chain.pointBlock
	chain.submitted = []*iconsensus.IConsensusClaimSubmitted{peer}
	repo.On("UpdateEpochToStaged", mock.Anything, app.ID, epoch.Index, chain.pointBlock).Return(nil).Once()
	result := runIORScan(t.Context(), s, claimStagePage, app, epoch, chain.head)
	require.ErrorIs(t, result.err, scanErr)
	require.Equal(t, 1, result.progress)
	require.Nil(t, epoch.ClaimTransactionHash)
	repo.AssertNotCalled(t, "RefreshClaimObservation", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}

func (c *iorScanTestChain) claimTransactionCanonical(ctx context.Context, _ common.Hash, _ uint64) (bool, error) {
	return !c.orphanedTransaction, ctx.Err()
}

func (c *iorScanTestChain) claimReference(ctx context.Context, number uint64) (txsender.BlockRef, error) {
	header := deliveryHeader(number)
	if c.branchChanged {
		header.Extra = []byte("fork")
	}
	return txsender.BlockRef{Number: number, Hash: header.Hash()}, ctx.Err()
}

func TestIORPointFallbackRechecksOriginalHeadAfterReorg(t *testing.T) {
	s, repo, chain, app, epoch, logs := newIORScanFixture(t, claimSubmitPage)
	chain.pointStatus = claimStatusStaged
	chain.onScan = func(stream string) {
		if stream == iorAcceptedStream {
			chain.branchChanged = true
			chain.scanErrors[stream] = txsender.ErrNonCanonical
		}
	}
	result := runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head)
	require.ErrorIs(t, result.err, txsender.ErrNonCanonical)
	require.Zero(t, result.progress)
	require.Empty(t, repo.Calls, "a point from the removed original head must not be projected")
	require.Empty(t, claimerIORRecords(t, logs, claimerIORStage))
	require.Empty(t, s.iorScans)
}

func TestIORCancellationDuringEndReferenceDoesNotSavePrefix(t *testing.T) {
	s, _, chain, app, epoch, _ := newIORScanFixture(t, claimSubmitPage)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	chain.ignoreReferenceCancellation = true
	chain.referenceError = func(txsender.BlockRef) error {
		cancel()
		return nil // A successful RPC response can race with cancellation.
	}
	result := runIORScan(ctx, s, claimSubmitPage, app, epoch, chain.head)
	require.ErrorIs(t, result.err, context.Canceled)
	require.Empty(t, s.iorScans)
}

func TestIORSubmittedProvisionalAuditReplaysAndFindsEarlierLocalVote(t *testing.T) {
	for _, policy := range []model.DefaultBlock{model.DefaultBlock_Safe, model.DefaultBlock_Latest} {
		for _, orphaned := range []bool{false, true} {
			t.Run(policy.String()+"/"+map[bool]string{false: "missing hash", true: "orphaned hash"}[orphaned], func(t *testing.T) {
				s, repo, chain, app, epoch, _ := newIORScanFixture(t, claimStagePage)
				s.defaultBlock, app.ConsensusType = policy, model.Consensus_Quorum
				chain.orphanedTransaction = orphaned
				if !orphaned {
					epoch.ClaimTransactionHash = nil
				}
				require.NoError(t, runIORScan(t.Context(), s, claimStagePage, app, epoch, chain.head).err)
				initial := append([]iorScanWindow(nil), chain.windows...)
				require.Equal(t, []iorScanWindow{
					{iorStagedStream, epoch.LastBlock + 1, chain.head},
					{iorSubmittedStream, epoch.LastBlock + 1, chain.head},
				}, initial)
				require.NoError(t, runIORScan(t.Context(), s, claimStagePage, app, epoch, chain.head).err)
				require.Equal(t, append(initial, initial...), chain.windows)
				require.Empty(t, s.iorScans)
				chain.head++
				scanErr := errors.New("audit vote logs unavailable")
				chain.scanErrors[iorSubmittedStream] = scanErr
				result := runIORScan(t.Context(), s, claimStagePage, app, epoch, chain.head)
				require.ErrorIs(t, result.err, scanErr)
				require.Empty(t, s.iorScans)
				delete(chain.scanErrors, iorSubmittedStream)
				before := len(chain.windows)
				require.NoError(t, runIORScan(t.Context(), s, claimStagePage, app, epoch, chain.head).err)
				require.Len(t, chain.windows, before+2)
				for _, window := range chain.windows[before:] {
					require.Equal(t, epoch.LastBlock+1, window.from, "failed provisional audit must remain fully retryable")
				}
				chain.branchChanged = true
				chain.head++
				local := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash("0xbeef"))
				local.Submitter, local.Raw.BlockNumber = common.HexToAddress("0x1"), iorScanTestStage
				chain.submitted = []*iconsensus.IConsensusClaimSubmitted{local}
				repo.On("RefreshClaimObservation", mock.Anything, epoch, new(local.Raw.TxHash), (*uint64)(nil)).Return(nil).Once()
				before = len(chain.windows)
				require.NoError(t, runIORScan(t.Context(), s, claimStagePage, app, epoch, chain.head).err)
				for _, window := range chain.windows[before:] {
					require.Equal(t, epoch.LastBlock+1, window.from)
				}
				require.Equal(t, local.Raw.TxHash, *epoch.ClaimTransactionHash)
				require.Empty(t, s.iorScans)
				repo.AssertExpectations(t)
			})
		}
	}
}

func TestIORInvalidPointClockNeverProjects(t *testing.T) {
	invalid := map[string]*big.Int{
		"nil":      nil,
		"negative": big.NewInt(-1),
		"overflow": new(big.Int).Lsh(big.NewInt(1), 64),
		"future":   new(big.Int).SetUint64(iorScanTestHead + 1),
	}
	for name, clock := range invalid {
		t.Run(name, func(t *testing.T) {
			s, repo, _, app, epoch, logs := newIORScanFixture(t, claimSubmitPage)
			claim := makeClaimStatus(claimStatusStaged, epoch, iorScanTestStage)
			claim.StagingBlockNumber = clock
			_, err := s.updateEpochStagedFromClaimStatus(t.Context(), app, epoch, claim, "clock test", iorScanTestHead)
			require.Error(t, err)
			require.Empty(t, repo.Calls)
			require.Empty(t, claimerIORRecords(t, logs, claimerIORStage))
		})
	}
}

func TestIORPointReadFailureDoesNotAcknowledgeOrForeclose(t *testing.T) {
	s, repo, chain, app, epoch, _ := newIORScanFixture(t, claimSubmitPage)
	app.ForecloseBlock = iorScanTestStage
	chain.pointErr = errors.New("point state unknown")
	result := runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head)
	require.ErrorIs(t, result.err, chain.pointErr)
	require.Empty(t, repo.Calls)
	require.Empty(t, s.iorScans)
}

func TestIORForeignQuorumVoteContextDoesNotRepeatOldVotes(t *testing.T) {
	s, _, chain, app, epoch, logs := newIORScanFixture(t, claimSubmitPage)
	app.ConsensusType = model.Consensus_Quorum
	vote := makeSubmittedEventWithTxHash(app, epoch, common.HexToHash("0xbeef"))
	vote.Submitter, vote.Raw.BlockNumber = common.HexToAddress("0x2"), chain.head
	chain.submitted = []*iconsensus.IConsensusClaimSubmitted{vote}
	records := func() []map[string]any {
		t.Helper()
		decoder := json.NewDecoder(bytes.NewReader(logs.Bytes()))
		var selected []map[string]any
		for decoder.More() {
			var record map[string]any
			require.NoError(t, decoder.Decode(&record))
			if record["invalid_outputs_root"] != nil {
				selected = append(selected, record)
			}
		}
		return selected
	}
	require.NoError(t, runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head).err)
	require.Len(t, records(), 1)
	require.Equal(t, "INFO", records()[0]["level"])
	require.Equal(t, true, records()[0]["invalid_outputs_root"])
	require.Equal(t, float64(epoch.Index), records()[0]["epoch_index"])
	require.NoError(t, runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head).err)
	require.Len(t, records(), 1)
	chain.head++
	next := *vote
	next.Raw.BlockNumber = chain.head
	chain.submitted = append(chain.submitted, &next)
	require.NoError(t, runIORScan(t.Context(), s, claimSubmitPage, app, epoch, chain.head).err)
	require.Len(t, records(), 2, "a new vote must remain visible")
}
