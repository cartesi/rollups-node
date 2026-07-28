// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrNotFound = errors.New("not found")
	ErrNoUpdate = errors.New("update did not take effect")

	// ErrInputLogIdentityConflict indicates an input insert conflicted with a
	// stored row on the L1 log identity (transaction_hash, log_index) under a
	// different input index. On a canonical chain this cannot happen; it means
	// the stored inputs diverged from rescanned chain data (e.g. a reorg past
	// the input cursor). Retrying cannot succeed — callers should treat the
	// application state as corrupted.
	ErrInputLogIdentityConflict = errors.New("input L1 log identity conflicts with stored data")
)

type Pagination struct {
	Limit  uint64
	Offset uint64
}

type ApplicationFilter struct {
	Enabled          *bool
	Status           *ApplicationStatus
	Statuses         []ApplicationStatus
	DataAvailability *DataAvailabilitySelector
	ConsensusType    *Consensus
	ConsensusTypes   []Consensus
	// ForeclosureRecorded filters by the foreclose_block column: when non-nil
	// and true, returns only apps whose foreclosure has been observed and
	// recorded by the evmreader; when non-nil and false, returns only apps
	// without a recorded foreclosure.
	ForeclosureRecorded *bool
}

// ExecutableApplicationsFilter selects apps that may run normal machine work.
//
// This is the repository-side equivalent of Application.CanExecute. Keep this
// helper shared because manager and validator both need the exact same
// database predicate before they create machines or compute claims.
func ExecutableApplicationsFilter() ApplicationFilter {
	return ApplicationFilter{
		Enabled:             new(true),
		Status:              new(ApplicationStatus_OK),
		ForeclosureRecorded: new(false),
	}
}

type EpochFilter struct {
	Status      []EpochStatus
	BeforeBlock *uint64
	IndexRange  *Range
}

type InputFilter struct {
	EpochIndex      *uint64
	Status          *InputCompletionStatus
	NotStatus       *InputCompletionStatus
	Sender          *common.Address
	TransactionHash *common.Hash
	IndexRange      *Range
}

type Range struct {
	Start uint64
	End   uint64
}

type OutputFilter struct {
	EpochIndex     *uint64
	InputIndex     *uint64
	BlockRange     *Range
	IndexRange     *Range
	OutputType     *[][]byte
	Executed       *bool
	VoucherAddress *common.Address
}

type ReportFilter struct {
	EpochIndex *uint64
	InputIndex *uint64
	IndexRange *Range
}

type StateHashFilter struct {
	EpochIndex *uint64
}

type TournamentFilter struct {
	EpochIndex              *uint64
	Level                   *uint64
	ParentTournamentAddress *common.Address
	ParentMatchIDHash       *common.Hash
}

type CommitmentFilter struct {
	EpochIndex        *uint64
	TournamentAddress *string
}

type MatchFilter struct {
	EpochIndex        *uint64
	TournamentAddress *string
}

type WithdrawalFilter struct {
	AccountIndex *uint64
}

type ApplicationRepository interface {
	CreateApplication(ctx context.Context, app *Application, withExecutionParameters bool) (int64, error)
	GetApplication(ctx context.Context, nameOrAddress string) (*Application, error)
	GetProcessedInputCount(ctx context.Context, nameOrAddress string) (uint64, error)
	UpdateApplication(ctx context.Context, app *Application) error
	UpdateApplicationEnabled(ctx context.Context, appID int64, enabled bool) error
	EnableApplicationAndClearFailed(ctx context.Context, appID int64) error
	UpdateApplicationStatus(ctx context.Context, appID int64, status ApplicationStatus, reason *string) error
	// UpdateApplicationForeclosure records the one-shot Foreclosure() event
	// and advances the foreclosure scan cursor in
	// one transaction. Used when the evmreader found the event in the scanned
	// window; after this marker is recorded the scanner stops checking the app.
	UpdateApplicationForeclosure(
		ctx context.Context,
		appID int64,
		block uint64,
		txHash common.Hash,
		blockNumber uint64,
	) error
	// UpdateApplicationLastForecloseCheckBlock advances the highest block
	// the Foreclosure-event log search has scanned. The write is strictly
	// monotonic: a lower or equal blockNumber is a no-op, so out-of-order
	// ticks cannot rewind the value and re-cause a long [deployment, head]
	// rescan.
	UpdateApplicationLastForecloseCheckBlock(ctx context.Context, appID int64, blockNumber uint64) error
	// UpdateAccountsDriveProved records the one-shot
	// AccountsDriveMerkleRootProved event and advances the scan cursor
	// in one transaction. Used when the evmreader found the event in the
	// scanned window; after this marker is recorded the scanner stops checking
	// the app and moves on to withdrawals.
	UpdateAccountsDriveProved(
		ctx context.Context,
		appID int64,
		block uint64,
		txHash common.Hash,
		root common.Hash,
		blockNumber uint64,
	) error
	// UpdateApplicationLastAccountsDriveProvedCheckBlock advances the highest
	// block the accounts-drive-proved scan has examined.
	// Strictly monotonic — out-of-order or duplicate ticks are silent no-ops.
	UpdateApplicationLastAccountsDriveProvedCheckBlock(ctx context.Context, appID int64, blockNumber uint64) error
	DeleteApplication(ctx context.Context, id int64) error
	ListApplications(ctx context.Context, f ApplicationFilter, p Pagination, descending bool) ([]*Application, uint64, error)

	GetExecutionParameters(ctx context.Context, applicationID int64) (*ExecutionParameters, error)
	UpdateExecutionParameters(ctx context.Context, ep *ExecutionParameters) error

	GetEventLastCheckBlock(ctx context.Context, appID int64, event MonitoredEvent) (uint64, error)
	UpdateEventLastCheckBlock(ctx context.Context, appIDs []int64, event MonitoredEvent, blockNumber uint64) error

	GetLastSnapshot(ctx context.Context, nameOrAddress string) (*Input, error)
}

type EpochRepository interface {
	// FIXME move to BulkOperationsRepository
	CreateEpochsAndInputs(ctx context.Context, nameOrAddress string, epochInputMap map[*Epoch][]*Input, blockNumber uint64) error

	GetEpoch(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)
	GetLastAcceptedEpochIndex(ctx context.Context, nameOrAddress string) (uint64, error)
	GetLastNonOpenEpoch(ctx context.Context, nameOrAddress string) (*Epoch, error)
	GetEpochByVirtualIndex(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)

	UpdateEpochClaimTransactionHash(ctx context.Context, nameOrAddress string, e *Epoch) error
	UpdateEpochStatus(ctx context.Context, nameOrAddress string, e *Epoch) error
	UpdateEpochInputsProcessed(ctx context.Context, nameOrAddress string, epochIndex uint64) error
	UpdateEpochOutputsProof(ctx context.Context, appID int64, epochIndex uint64, proof *OutputsProof) error
	RepeatPreviousEpochOutputsProof(ctx context.Context, appID int64, epochIndex uint64) error

	ListEpochs(ctx context.Context, nameOrAddress string, f EpochFilter, p Pagination, descending bool) ([]*Epoch, uint64, error)

	// HasUndrainedEpochsBeforeBlock reports whether any input for the given
	// application has block_number <= blockBound and is still unprocessed
	// in a non-terminal epoch.
	// The bound is inclusive because a valid InputAdded event in the same
	// block as Foreclosure must have executed before the foreclosure
	// transaction; post-foreclosure addInput calls revert and emit no event.
	// PRT uses this gate to keep the post-foreclosure drain pending until
	// the advancer has processed every pre-foreclosure input — it does NOT
	// wait for CLAIM_ACCEPTED because PRT tournaments cannot settle on a
	// foreclosed IApplication, so waiting would stall forever.
	HasUndrainedEpochsBeforeBlock(ctx context.Context, appID int64, blockBound uint64) (bool, error)

	// HasUnreconciledClaimsBeforeBlock is the broader gate used by the
	// Authority/Quorum claimer: it returns true while any epoch for the
	// given application is still in OPEN/CLOSED/INPUTS_PROCESSED OR in
	// CLAIM_COMPUTED/CLAIM_SUBMITTED/CLAIM_STAGED with FirstBlock <=
	// blockBound. The extra states cover the new-node-bootstrap path
	// (a fresh DB entry for an already-foreclosed contract): each
	// pre-foreclosure on-chain-accepted claim must be mirrored to
	// CLAIM_ACCEPTED locally, or marked CLAIM_FORECLOSED when the contract
	// state proves acceptance is impossible, before the app is considered
	// drained. Otherwise downstream tooling sees a final state that diverges
	// from chain reality.
	HasUnreconciledClaimsBeforeBlock(ctx context.Context, appID int64, blockBound uint64) (bool, error)

	// ForecloseUnacceptedEpochsAtOrAfterBlock marks Authority/Quorum epochs
	// that overlap the foreclosure block as CLAIM_FORECLOSED. Such epochs
	// cannot have an accepted on-chain claim because claim submission and
	// acceptance are blocked once foreclosure is observed, and a claim whose
	// last processed block is the foreclosure block cannot be accepted in that
	// same block.
	ForecloseUnacceptedEpochsAtOrAfterBlock(ctx context.Context, appID int64, blockBound uint64) (int64, error)
}

type InputRepository interface {
	GetInput(ctx context.Context, nameOrAddress string, inputIndex uint64) (*Input, error)
	GetLastInput(ctx context.Context, appAddress string, epochIndex uint64) (*Input, error)
	GetLastProcessedInput(ctx context.Context, appAddress string) (*Input, error)
	ListInputs(ctx context.Context, nameOrAddress string, f InputFilter, p Pagination, descending bool) ([]*Input, uint64, error)
	GetNumberOfInputs(ctx context.Context, nameOrAddress string) (uint64, error)
	UpdateInputSnapshotURI(ctx context.Context, appId int64, inputIndex uint64, snapshotURI string) error
}

type OutputRepository interface {
	GetOutput(ctx context.Context, nameOrAddress string, outputIndex uint64) (*Output, error)
	UpdateOutputsExecution(ctx context.Context, nameOrAddress string, executedOutputs []*Output, blockNumber uint64) error
	ListOutputs(ctx context.Context, nameOrAddress string, f OutputFilter, p Pagination, descending bool) ([]*Output, uint64, error)
	GetNumberOfExecutedOutputs(ctx context.Context, nameOrAddress string) (uint64, error)
	GetNumberOfPendingExecutableOutputs(ctx context.Context, nameOrAddress string) (uint64, error)
}

type ReportRepository interface {
	GetReport(ctx context.Context, nameOrAddress string, reportIndex uint64) (*Report, error)
	ListReports(ctx context.Context, nameOrAddress string, f ReportFilter, p Pagination, descending bool) ([]*Report, uint64, error)
}

type WithdrawalRepository interface {
	// InsertWithdrawal records a Withdrawal(uint64 accountIndex, bytes account,
	// bytes output) event observed on chain. Idempotent on the (application_id,
	// account_index) primary key via ON CONFLICT DO NOTHING: re-processing the
	// same block on restart cannot fail and cannot diverge from the first write.
	// The contract marks each account index as withdrawn (see
	// IApplication.wereAccountFundsWithdrawn), so the event fires at most once
	// per slot per app — second observations are always restart artifacts.
	InsertWithdrawal(ctx context.Context, w *Withdrawal) error

	// StoreWithdrawalEvents records Withdrawal events from a completed scanner
	// window and advances the
	// application's last_withdrawal_check_block in the same database
	// transaction. This keeps the local withdrawal count and scanner cursor in
	// sync: either both reflect the scanned window, or neither does.
	StoreWithdrawalEvents(
		ctx context.Context,
		appID int64,
		withdrawals []*Withdrawal,
		blockNumber uint64,
	) error

	// GetNumberOfWithdrawals returns the number of Withdrawal rows stored for
	// an application. The post-foreclosure scanner uses it as the local
	// previous counter when resuming after last_withdrawal_check_block.
	GetNumberOfWithdrawals(ctx context.Context, appID int64) (uint64, error)

	// GetWithdrawal returns a single withdrawal by (application, accountIndex).
	// Returns (nil, nil) when the row does not exist — mirrors GetOutput and
	// the project convention for Get* lookups; the JSON-RPC layer turns the
	// nil into a resource-not-found error code. Application is identified by
	// name or address.
	GetWithdrawal(ctx context.Context, nameOrAddress string, accountIndex uint64) (*Withdrawal, error)

	// ListWithdrawals returns a paginated list for an application, optionally
	// filtered by account_index. nameOrAddress + pagination/ordering shape
	// mirror ListOutputs: default ascending by account_index, descending=true
	// reverses it.
	ListWithdrawals(ctx context.Context, nameOrAddress string, f WithdrawalFilter, p Pagination, descending bool) ([]*Withdrawal, uint64, error)
}

type StateHashRepository interface {
	ListStateHashes(ctx context.Context, nameOrAddress string, f StateHashFilter, p Pagination, descending bool) ([]*StateHash, uint64, error)
}

type TournamentRepository interface {
	CreateTournament(ctx context.Context, nameOrAddress string, t *Tournament) error
	UpdateTournament(ctx context.Context, nameOrAddress string, t *Tournament) error
	GetTournament(ctx context.Context, nameOrAddress string, address string) (*Tournament, error)
	ListTournaments(ctx context.Context, nameOrAddress string, f TournamentFilter,
		p Pagination, descending bool) ([]*Tournament, uint64, error)
}

type CommitmentRepository interface {
	CreateCommitment(ctx context.Context, nameOrAddress string, c *Commitment) error
	GetCommitment(ctx context.Context, nameOrAddress string, epochIndex uint64, tournamentAddress string, commitmentHex string) (*Commitment, error)
	ListCommitments(ctx context.Context, nameOrAddress string, f CommitmentFilter, p Pagination, descending bool) ([]*Commitment, uint64, error)
}

type MatchRepository interface {
	CreateMatch(ctx context.Context, nameOrAddress string, m *Match) error
	UpdateMatch(ctx context.Context, nameOrAddress string, m *Match) error
	GetMatch(ctx context.Context, nameOrAddress string, epochIndex uint64, tournamentAddress string, idHashHex string) (*Match, error)
	ListMatches(ctx context.Context, nameOrAddress string, f MatchFilter, p Pagination, descending bool) ([]*Match, uint64, error)
}

type MatchAdvancedRepository interface {
	CreateMatchAdvanced(ctx context.Context, nameOrAddress string, m *MatchAdvanced) error
	GetMatchAdvanced(ctx context.Context, nameOrAddress string, epochIndex uint64, tournamentAddress string, idHashHex string, parentHex string) (*MatchAdvanced, error)
	ListMatchAdvances(ctx context.Context, nameOrAddress string, epochIndex uint64, tournamentAddress string, idHashHex string, p Pagination, descending bool) ([]*MatchAdvanced, uint64, error)
}

type BulkOperationsRepository interface {
	StoreAdvanceResult(ctx context.Context, appID int64, result *AdvanceResult) error
	StoreClaimAndProofs(ctx context.Context, epoch *Epoch, outputs []*Output) error
	StoreTournamentEvents(ctx context.Context, appID int64, commitments []*Commitment, matches []*Match,
		matchAdvanced []*MatchAdvanced, matchDeleted []*Match, lastBlock uint64) error
}

type NodeConfigRepository interface {
	SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error
	LoadNodeConfigRaw(ctx context.Context, key string) (rawJSON []byte, createdAt, updatedAt time.Time, err error)
}

// TODO: migrate ClaimRow -> Application + Epoch and use the other interfaces
type ClaimerRepository interface {
	SelectClaimsToSubmitPerApp(ctx context.Context) (
		map[int64]*Epoch,
		map[int64]*Epoch,
		map[int64]*Application,
		error,
	)
	SelectClaimsToStagePerApp(ctx context.Context) (
		map[int64]*Epoch,
		map[int64]*Epoch,
		map[int64]*Application,
		error,
	)
	SelectClaimsToAcceptPerApp(ctx context.Context) (
		map[int64]*Epoch,
		map[int64]*Epoch,
		map[int64]*Application,
		error,
	)
	UpdateEpochWithSubmittedClaim(
		ctx context.Context,
		applicationID int64,
		index uint64,
		transactionHash common.Hash,
	) error
	UpdateEpochToStaged(
		ctx context.Context,
		applicationID int64,
		index uint64,
		stagedAtBlock uint64,
	) error
	UpdateEpochThroughStaging(
		ctx context.Context,
		applicationID int64,
		index uint64,
		transactionHash common.Hash,
		stagedAtBlock uint64,
	) error
	UpdateEpochReconciledStaged(
		ctx context.Context,
		applicationID int64,
		index uint64,
		stagedAtBlock uint64,
	) error
	// UpdateEpochWithAcceptedClaim transitions an epoch to CLAIM_ACCEPTED.
	// txHash is optional: pass non-nil to record claim_transaction_hash
	// (catch-up reconciliations where the epoch never went through the
	// CLAIM_SUBMITTED transition); pass nil to leave the column untouched
	// (the normal-flow case where the column was populated during
	// CLAIM_SUBMITTED).
	UpdateEpochWithAcceptedClaim(
		ctx context.Context,
		applicationID int64,
		index uint64,
		txHash *common.Hash,
	) error
	// UpdateEpochWithForeclosedClaim transitions a single epoch's non-terminal
	// claim status to CLAIM_FORECLOSED after application foreclosure makes the
	// claim path impossible. Keyed by (application, epoch index); applies to any
	// consensus type.
	UpdateEpochWithForeclosedClaim(
		ctx context.Context,
		applicationID int64,
		index uint64,
	) error
	// RejectEpochAndSetApplicationDiverged atomically marks an epoch as
	// CLAIM_REJECTED and the application as DIVERGED. Used when Quorum
	// consensus stages or accepts a different claim before the local claim has
	// staged, making the local claim unreachable. The application is always
	// halted, even when no epoch row matched the reject.
	RejectEpochAndSetApplicationDiverged(
		ctx context.Context,
		applicationID int64,
		index uint64,
		reason string,
	) error
}

type Repository interface {
	ReplayRepository
	ApplicationRepository
	EpochRepository
	InputRepository
	OutputRepository
	ReportRepository
	StateHashRepository
	TournamentRepository
	CommitmentRepository
	MatchRepository
	MatchAdvancedRepository
	BulkOperationsRepository
	NodeConfigRepository
	ClaimerRepository
	WithdrawalRepository
	Close()
}

func SaveNodeConfig[T any](
	ctx context.Context,
	repo NodeConfigRepository,
	nc *NodeConfig[T],
) error {
	data, err := json.Marshal(nc.Value)
	if err != nil {
		return fmt.Errorf("marshal node_config value failed: %w", err)
	}
	err = repo.SaveNodeConfigRaw(ctx, nc.Key, data)
	if err != nil {
		return err
	}
	return nil
}

func LoadNodeConfig[T any](
	ctx context.Context,
	repo NodeConfigRepository,
	key string,
) (*NodeConfig[T], error) {
	raw, createdAt, updatedAt, err := repo.LoadNodeConfigRaw(ctx, key)
	if err != nil || raw == nil {
		return nil, err
	}
	var val T
	if err := json.Unmarshal(raw, &val); err != nil {
		return nil, fmt.Errorf("unmarshal node_config value failed: %w", err)
	}
	return &NodeConfig[T]{
		Key:       key,
		Value:     val,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}
