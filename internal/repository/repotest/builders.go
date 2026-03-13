// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repotest

import (
	"context"
	"fmt"
	"math/big"
	"sync/atomic"
	"testing"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

// counter provides unique values across all builders to avoid collisions.
var counter atomic.Uint64

func nextID() uint64 {
	return counter.Add(1)
}

// UniqueAddress returns a unique Ethereum address for test isolation.
func UniqueAddress() common.Address {
	return common.BigToAddress(big.NewInt(int64(nextID())))
}

// UniqueHash returns a unique hash for test isolation.
func UniqueHash() common.Hash {
	return common.BigToHash(big.NewInt(int64(nextID())))
}

// ---------------------------------------------------------------------------
// ApplicationBuilder
// ---------------------------------------------------------------------------

type ApplicationBuilder struct {
	app                     *Application
	withExecutionParameters bool
}

func NewApplicationBuilder() *ApplicationBuilder {
	id := nextID()
	return &ApplicationBuilder{
		app: &Application{
			Name:                fmt.Sprintf("test-app-%d", id),
			IApplicationAddress: UniqueAddress(),
			IConsensusAddress:   UniqueAddress(),
			IInputBoxAddress:    UniqueAddress(),
			TemplateHash:        UniqueHash(),
			TemplateURI:         fmt.Sprintf("/template/%d", id),
			EpochLength:         10,
			DataAvailability:    DataAvailability_InputBox[:],
			ConsensusType:       Consensus_Authority,
			State:               ApplicationState_Enabled,
		},
	}
}

func (b *ApplicationBuilder) WithName(name string) *ApplicationBuilder {
	b.app.Name = name
	return b
}

func (b *ApplicationBuilder) WithAddress(addr common.Address) *ApplicationBuilder {
	b.app.IApplicationAddress = addr
	return b
}

func (b *ApplicationBuilder) WithConsensus(c Consensus) *ApplicationBuilder {
	b.app.ConsensusType = c
	return b
}

func (b *ApplicationBuilder) WithState(s ApplicationState) *ApplicationBuilder {
	b.app.State = s
	return b
}

func (b *ApplicationBuilder) WithEpochLength(l uint64) *ApplicationBuilder {
	b.app.EpochLength = l
	return b
}

func (b *ApplicationBuilder) WithDataAvailability(da []byte) *ApplicationBuilder {
	b.app.DataAvailability = da
	return b
}

func (b *ApplicationBuilder) WithExecutionParameters(ep ExecutionParameters) *ApplicationBuilder {
	b.app.ExecutionParameters = ep
	b.withExecutionParameters = true
	return b
}

// Build returns a copy of the Application model without persisting it.
func (b *ApplicationBuilder) Build() *Application {
	a := *b.app
	return &a
}

// Create persists the Application via the repository and returns a copy with the generated ID.
func (b *ApplicationBuilder) Create(
	ctx context.Context, t *testing.T, repo repository.Repository,
) *Application {
	t.Helper()
	a := *b.app
	id, err := repo.CreateApplication(ctx, &a, b.withExecutionParameters)
	require.NoError(t, err)
	a.ID = id
	return &a
}

// ---------------------------------------------------------------------------
// EpochBuilder
// ---------------------------------------------------------------------------

type EpochBuilder struct {
	epoch *Epoch
}

func NewEpochBuilder(appID int64) *EpochBuilder {
	return &EpochBuilder{
		epoch: &Epoch{
			ApplicationID: appID,
			Index:         0,
			VirtualIndex:  0,
			FirstBlock:    0,
			LastBlock:     9,
			Status:        EpochStatus_Open,
		},
	}
}

// WithIndex sets the epoch's physical Index.
// Note: VirtualIndex is auto-assigned by CreateEpochsAndInputs.
func (b *EpochBuilder) WithIndex(i uint64) *EpochBuilder {
	b.epoch.Index = i
	return b
}

func (b *EpochBuilder) WithStatus(s EpochStatus) *EpochBuilder {
	b.epoch.Status = s
	return b
}

func (b *EpochBuilder) WithBlocks(first, last uint64) *EpochBuilder {
	b.epoch.FirstBlock = first
	b.epoch.LastBlock = last
	return b
}

func (b *EpochBuilder) WithInputBounds(lower, upper uint64) *EpochBuilder {
	b.epoch.InputIndexLowerBound = lower
	b.epoch.InputIndexUpperBound = upper
	return b
}

func (b *EpochBuilder) WithClaimHash(h common.Hash) *EpochBuilder {
	b.epoch.OutputsMerkleRoot = &h
	return b
}

func (b *EpochBuilder) WithClaimTransactionHash(h common.Hash) *EpochBuilder {
	b.epoch.ClaimTransactionHash = &h
	return b
}

func (b *EpochBuilder) WithMachineHash(h common.Hash) *EpochBuilder {
	b.epoch.MachineHash = &h
	return b
}

// Build returns a copy of the Epoch model without persisting it.
func (b *EpochBuilder) Build() *Epoch {
	e := *b.epoch
	return &e
}

// ---------------------------------------------------------------------------
// InputBuilder
// ---------------------------------------------------------------------------

type InputBuilder struct {
	input *Input
}

func NewInputBuilder() *InputBuilder {
	return &InputBuilder{
		input: &Input{
			Index:                0,
			BlockNumber:          1,
			RawData:              []byte("input-data"),
			Status:               InputCompletionStatus_None,
			TransactionReference: UniqueHash(),
		},
	}
}

func (b *InputBuilder) WithIndex(i uint64) *InputBuilder {
	b.input.Index = i
	return b
}

func (b *InputBuilder) WithEpochApplicationID(id int64) *InputBuilder {
	b.input.EpochApplicationID = id
	return b
}

func (b *InputBuilder) WithEpochIndex(i uint64) *InputBuilder {
	b.input.EpochIndex = i
	return b
}

func (b *InputBuilder) WithBlockNumber(n uint64) *InputBuilder {
	b.input.BlockNumber = n
	return b
}

func (b *InputBuilder) WithStatus(s InputCompletionStatus) *InputBuilder {
	b.input.Status = s
	return b
}

func (b *InputBuilder) WithRawData(data []byte) *InputBuilder {
	b.input.RawData = data
	return b
}

func (b *InputBuilder) WithTransactionReference(h common.Hash) *InputBuilder {
	b.input.TransactionReference = h
	return b
}

// Build returns a copy of the Input model without persisting it.
func (b *InputBuilder) Build() *Input {
	i := *b.input
	return &i
}

// ---------------------------------------------------------------------------
// TournamentBuilder
// ---------------------------------------------------------------------------

type TournamentBuilder struct {
	tournament *Tournament
}

func NewTournamentBuilder(appID int64) *TournamentBuilder {
	return &TournamentBuilder{
		tournament: &Tournament{
			ApplicationID: appID,
			EpochIndex:    0,
			Address:       UniqueAddress(),
			MaxLevel:      3,
			Level:         0,
			Log2Step:      20,
			Height:        2,
		},
	}
}

func (b *TournamentBuilder) WithEpochIndex(i uint64) *TournamentBuilder {
	b.tournament.EpochIndex = i
	return b
}

func (b *TournamentBuilder) WithAddress(addr common.Address) *TournamentBuilder {
	b.tournament.Address = addr
	return b
}

func (b *TournamentBuilder) WithLevel(l uint64) *TournamentBuilder {
	b.tournament.Level = l
	return b
}

func (b *TournamentBuilder) WithParent(addr common.Address, matchIDHash common.Hash) *TournamentBuilder {
	b.tournament.ParentTournamentAddress = &addr
	b.tournament.ParentMatchIDHash = &matchIDHash
	return b
}

// Build returns a copy of the Tournament model without persisting it.
func (b *TournamentBuilder) Build() *Tournament {
	t := *b.tournament
	return &t
}

// ---------------------------------------------------------------------------
// CommitmentBuilder
// ---------------------------------------------------------------------------

type CommitmentBuilder struct {
	commitment *Commitment
}

func NewCommitmentBuilder(appID int64) *CommitmentBuilder {
	return &CommitmentBuilder{
		commitment: &Commitment{
			ApplicationID:     appID,
			EpochIndex:        0,
			TournamentAddress: UniqueAddress(),
			Commitment:        UniqueHash(),
			FinalStateHash:    UniqueHash(),
			SubmitterAddress:  UniqueAddress(),
			BlockNumber:       100,
			TxHash:            UniqueHash(),
		},
	}
}

func (b *CommitmentBuilder) WithEpochIndex(i uint64) *CommitmentBuilder {
	b.commitment.EpochIndex = i
	return b
}

func (b *CommitmentBuilder) WithTournamentAddress(addr common.Address) *CommitmentBuilder {
	b.commitment.TournamentAddress = addr
	return b
}

func (b *CommitmentBuilder) WithCommitmentHash(h common.Hash) *CommitmentBuilder {
	b.commitment.Commitment = h
	return b
}

// Build returns a copy of the Commitment model without persisting it.
func (b *CommitmentBuilder) Build() *Commitment {
	c := *b.commitment
	return &c
}

// ---------------------------------------------------------------------------
// MatchBuilder
// ---------------------------------------------------------------------------

type MatchBuilder struct {
	match *Match
}

func NewMatchBuilder(appID int64) *MatchBuilder {
	return &MatchBuilder{
		match: &Match{
			ApplicationID:     appID,
			EpochIndex:        0,
			TournamentAddress: UniqueAddress(),
			IDHash:            UniqueHash(),
			CommitmentOne:     UniqueHash(),
			CommitmentTwo:     UniqueHash(),
			LeftOfTwo:         UniqueHash(),
			BlockNumber:       100,
			TxHash:            UniqueHash(),
			Winner:            WinnerCommitment_NONE,
			DeletionReason:    MatchDeletionReason_NOT_DELETED,
		},
	}
}

func (b *MatchBuilder) WithEpochIndex(i uint64) *MatchBuilder {
	b.match.EpochIndex = i
	return b
}

func (b *MatchBuilder) WithTournamentAddress(addr common.Address) *MatchBuilder {
	b.match.TournamentAddress = addr
	return b
}

func (b *MatchBuilder) WithIDHash(h common.Hash) *MatchBuilder {
	b.match.IDHash = h
	return b
}

func (b *MatchBuilder) WithWinner(w WinnerCommitment) *MatchBuilder {
	b.match.Winner = w
	return b
}

func (b *MatchBuilder) WithCommitmentOne(h common.Hash) *MatchBuilder {
	b.match.CommitmentOne = h
	return b
}

func (b *MatchBuilder) WithCommitmentTwo(h common.Hash) *MatchBuilder {
	b.match.CommitmentTwo = h
	return b
}

func (b *MatchBuilder) WithDeletionReason(r MatchDeletionReason) *MatchBuilder {
	b.match.DeletionReason = r
	return b
}

// Build returns a copy of the Match model without persisting it.
func (b *MatchBuilder) Build() *Match {
	m := *b.match
	return &m
}

// ---------------------------------------------------------------------------
// MatchAdvancedBuilder
// ---------------------------------------------------------------------------

type MatchAdvancedBuilder struct {
	ma *MatchAdvanced
}

func NewMatchAdvancedBuilder(appID int64) *MatchAdvancedBuilder {
	return &MatchAdvancedBuilder{
		ma: &MatchAdvanced{
			ApplicationID:     appID,
			EpochIndex:        0,
			TournamentAddress: UniqueAddress(),
			IDHash:            UniqueHash(),
			OtherParent:       UniqueHash(),
			LeftNode:          UniqueHash(),
			BlockNumber:       100,
			TxHash:            UniqueHash(),
		},
	}
}

func (b *MatchAdvancedBuilder) WithEpochIndex(i uint64) *MatchAdvancedBuilder {
	b.ma.EpochIndex = i
	return b
}

func (b *MatchAdvancedBuilder) WithTournamentAddress(addr common.Address) *MatchAdvancedBuilder {
	b.ma.TournamentAddress = addr
	return b
}

func (b *MatchAdvancedBuilder) WithIDHash(h common.Hash) *MatchAdvancedBuilder {
	b.ma.IDHash = h
	return b
}

func (b *MatchAdvancedBuilder) WithOtherParent(h common.Hash) *MatchAdvancedBuilder {
	b.ma.OtherParent = h
	return b
}

// Build returns a copy of the MatchAdvanced model without persisting it.
func (b *MatchAdvancedBuilder) Build() *MatchAdvanced {
	m := *b.ma
	return &m
}

// ---------------------------------------------------------------------------
// Seed — convenience for creating a minimum viable Application + Epoch + Input
// ---------------------------------------------------------------------------

// SeedResult holds the entities created by Seed.
type SeedResult struct {
	App   *Application
	Epoch *Epoch
	Input *Input
}

// AdvanceEpochStatus transitions an epoch through the valid state machine to
// the target status, finding the shortest path through the transition graph.
// The graph mirrors the SQL trigger enforce_epoch_status_transition:
//
//	OPEN → CLOSED → INPUTS_PROCESSED → CLAIM_COMPUTED
//	CLAIM_COMPUTED → CLAIM_SUBMITTED → CLAIM_ACCEPTED
//	CLAIM_COMPUTED → CLAIM_ACCEPTED  (PRT, sync catch-up, or reader-only mode)
//	CLAIM_SUBMITTED → CLAIM_REJECTED
func AdvanceEpochStatus(
	ctx context.Context, t *testing.T,
	repo repository.Repository,
	nameOrAddress string,
	epoch *Epoch,
	target EpochStatus,
) {
	t.Helper()

	// Adjacency list mirrors the SQL trigger's valid transitions.
	next := map[EpochStatus][]EpochStatus{
		EpochStatus_Open:            {EpochStatus_Closed},
		EpochStatus_Closed:          {EpochStatus_InputsProcessed},
		EpochStatus_InputsProcessed: {EpochStatus_ClaimComputed},
		EpochStatus_ClaimComputed:   {EpochStatus_ClaimSubmitted, EpochStatus_ClaimAccepted},
		EpochStatus_ClaimSubmitted:  {EpochStatus_ClaimAccepted, EpochStatus_ClaimRejected},
	}

	// BFS to find shortest valid path.
	type step struct {
		status EpochStatus
		path   []EpochStatus
	}
	queue := []step{{status: epoch.Status, path: nil}}
	visited := map[EpochStatus]bool{epoch.Status: true}

	var path []EpochStatus
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.status == target {
			path = cur.path
			break
		}
		for _, n := range next[cur.status] {
			if !visited[n] {
				visited[n] = true
				p := make([]EpochStatus, len(cur.path)+1)
				copy(p, cur.path)
				p[len(cur.path)] = n
				queue = append(queue, step{n, p})
			}
		}
	}
	if path == nil {
		t.Fatalf("AdvanceEpochStatus: no valid path from %s to %s",
			epoch.Status, target)
	}

	for _, s := range path {
		// The DB trigger requires proof fields to be non-null when
		// entering CLAIM_COMPUTED.  Populate them with dummy values
		// so tests that only care about status transitions don't need
		// to set up proofs manually.
		if s == EpochStatus_ClaimComputed {
			setDummyProofFields(ctx, t, repo, nameOrAddress, epoch)
			// For PRT apps StoreClaimAndProofs already set the status
			// to CLAIM_COMPUTED, so skip the redundant UpdateEpochStatus.
			app, err := repo.GetApplication(ctx, nameOrAddress)
			require.NoError(t, err)
			if app.IsDaveConsensus() {
				epoch.Status = s
				continue
			}
		}
		epoch.Status = s
		err := repo.UpdateEpochStatus(ctx, nameOrAddress, epoch)
		require.NoError(t, err)
	}
}

// setDummyProofFields populates the proof fields required by the DB trigger
// for the INPUTS_PROCESSED → CLAIM_COMPUTED transition.
// For all apps:  machine_hash, outputs_merkle_root, outputs_merkle_proof.
// For PRT apps:  additionally commitment and commitment_proof (set via
// StoreClaimAndProofs which also transitions the status atomically).
func setDummyProofFields(
	ctx context.Context, t *testing.T,
	repo repository.Repository,
	nameOrAddress string,
	epoch *Epoch,
) {
	t.Helper()

	proof := &OutputsProof{
		OutputsHash:      UniqueHash(),
		OutputsHashProof: [][32]byte{[32]byte(UniqueHash())},
		MachineHash:      UniqueHash(),
	}
	err := repo.UpdateEpochOutputsProof(
		ctx, epoch.ApplicationID, epoch.Index, proof)
	require.NoError(t, err)

	app, err := repo.GetApplication(ctx, nameOrAddress)
	require.NoError(t, err)
	if app.IsDaveConsensus() {
		commitHash := UniqueHash()
		epoch.Commitment = &commitHash
		epoch.CommitmentProof = []common.Hash{UniqueHash()}
		epoch.Status = EpochStatus_ClaimComputed
		err = repo.StoreClaimAndProofs(ctx, epoch, nil)
		require.NoError(t, err)
	}
}

// Seed creates and persists a minimal Application with one Epoch and one Input.
func Seed(ctx context.Context, t *testing.T, repo repository.Repository) *SeedResult {
	t.Helper()

	app := NewApplicationBuilder().Create(ctx, t, repo)

	epoch := NewEpochBuilder(app.ID).
		WithIndex(0).
		WithStatus(EpochStatus_Closed).
		WithBlocks(0, 9).
		WithInputBounds(0, 0).
		Build()

	input := NewInputBuilder().
		WithIndex(0).
		WithBlockNumber(5).
		Build()

	epochInputMap := map[*Epoch][]*Input{epoch: {input}}
	err := repo.CreateEpochsAndInputs(ctx, app.IApplicationAddress.String(), epochInputMap, 10)
	require.NoError(t, err)

	return &SeedResult{
		App:   app,
		Epoch: epoch,
		Input: input,
	}
}
