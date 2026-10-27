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
	// ErrLockTimeout means a database lock wait exceeded its transaction-local bound.
	ErrLockTimeout        = errors.New("database lock wait timed out")
	ErrNotFound           = errors.New("not found")
	ErrNoUpdate           = errors.New("update did not take effect")
	ErrApplicationEnabled = errors.New("application must be disabled before removal")
	// ErrEpochForeclosed means a stale epoch publication lost to foreclosure.
	// The stored epoch is CLAIM_FORECLOSED; no new claim or proof was stored.
	ErrEpochForeclosed = errors.New("epoch publication superseded by foreclosure")
	// ErrApplicationNotRunnable means an advance result cannot be stored because
	// the application's durable status does not allow machine execution.
	ErrApplicationNotRunnable = errors.New("application is not runnable")
	// ErrAdvanceCursorMismatch means an advance result does not match the next
	// unprocessed input selected by the application's durable cursor.
	ErrAdvanceCursorMismatch = errors.New("advance result does not match application cursor")
	// ErrAdvanceAfterTerminal preserves the more specific classification for a
	// store rejected because the application's durable status is terminal.
	ErrAdvanceAfterTerminal = errors.New("cannot store an advance result after a terminal input")
	// ErrInvalidStateProof means an advance result or epoch publication did not
	// include the complete three-leaf machine state proof.
	ErrInvalidStateProof = errors.New("invalid machine state proof")
	// ErrTournamentEventConflict means an observation contradicts an already
	// stored immutable tournament fact or would replace a newer snapshot.
	ErrTournamentEventConflict = errors.New("tournament observation conflicts with stored data")

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
	AfterID        *int64
	ThroughID      *int64
	Enabled        *bool
	Status         *ApplicationStatus
	Statuses       []ApplicationStatus
	ConsensusType  *Consensus
	ConsensusTypes []Consensus
	// ForeclosureRecorded filters by the foreclose_block column: when non-nil
	// and true, returns only apps whose foreclosure has been observed and
	// recorded by the evmreader; when non-nil and false, returns only apps
	// without a recorded foreclosure.
	ForeclosureRecorded *bool
}

// ExecutableApplicationsFilter selects apps that may run normal machine work.
// The manager uses this exact database predicate before creating machines.
func ExecutableApplicationsFilter() ApplicationFilter {
	return ApplicationFilter{
		Enabled:             new(true),
		Status:              new(ApplicationStatus_OK),
		ForeclosureRecorded: new(false),
	}
}

type EpochFilter struct {
	Status        []EpochStatus
	BeforeBlock   *uint64
	IndexRange    *Range
	HasTournament *bool
}

type InputFilter struct {
	EpochIndex      *uint64
	Status          *InputCompletionStatus
	NotStatus       *InputCompletionStatus
	Sender          *common.Address
	TransactionHash *common.Hash
	IndexRange      *Range
}

// Range defines a closed interval: both Start and End are inclusive.
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

type TournamentFilter struct {
	EpochIndex              *uint64
	Level                   *uint64
	ParentTournamentAddress *common.Address
	ParentMatchIDHash       *common.Hash
}

// RootBondRecoveryCandidate identifies an owned root bond in a published snapshot.
// Its current disposition must be checked on chain before sending a recovery.
type RootBondRecoveryCandidate struct {
	EpochIndex uint64
	Tournament common.Address
}

type CommitmentFilter struct {
	EpochIndex        *uint64
	TournamentAddress *string
}

type MatchFilter struct {
	EpochIndex        *uint64
	TournamentAddress *string
}

type BondEventFilter struct {
	EpochIndex        *uint64
	TournamentAddress *common.Address
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
	// UpdateEventLastCheckBlock advances the event cursor monotonically. Lower
	// or equal block numbers are no-ops.
	UpdateEventLastCheckBlock(ctx context.Context, appIDs []int64, event MonitoredEvent, blockNumber uint64) error

	GetLastSnapshot(ctx context.Context, nameOrAddress string) (*Input, error)
}

type EpochRepository interface {
	// FIXME move to BulkOperationsRepository
	CreateEpochsAndInputs(ctx context.Context, nameOrAddress string, epochInputMap map[*Epoch][]*Input, blockNumber uint64) error

	GetEpoch(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)
	GetEpochCompletionState(ctx context.Context, nameOrAddress string, index uint64) (*EpochCompletionState, error)
	StoreInvalidOutputsRoot(ctx context.Context, appID int64, epochIndex uint64, reason string) error
	GetLastAcceptedEpochIndex(ctx context.Context, nameOrAddress string) (uint64, error)
	GetLastNonOpenEpoch(ctx context.Context, nameOrAddress string) (*Epoch, error)
	GetLastNonOpenEpochIndex(ctx context.Context, nameOrAddress string) (*uint64, error)
	GetEpochByVirtualIndex(ctx context.Context, nameOrAddress string, index uint64) (*Epoch, error)

	UpdateEpochClaimTransactionHash(ctx context.Context, nameOrAddress string, e *Epoch) error
	UpdateEpochStatus(ctx context.Context, nameOrAddress string, e *Epoch) error
	UpdateEpochInputsProcessed(ctx context.Context, nameOrAddress string, epochIndex uint64, proof *StateProof) error

	ListEpochs(ctx context.Context, nameOrAddress string, f EpochFilter, p Pagination, descending bool) ([]*Epoch, uint64, error)

	// HasUndrainedEpochsBeforeBlock reports whether any input for the given
	// application has block_number <= blockBound and is still unprocessed
	// in a non-terminal epoch and is still executable. ET/IOR excludes NONE
	// inputs at or beyond processed_inputs, independently of the epoch cutoff.
	// The bound is inclusive because a valid InputAdded event in the same
	// block as Foreclosure must have executed before the foreclosure
	// transaction; post-foreclosure addInput calls revert and emit no event.
	// PRT uses this gate to keep the post-foreclosure drain pending until
	// the advancer has processed every executable pre-foreclosure input. It does NOT
	// wait for CLAIM_ACCEPTED because PRT tournaments cannot settle on a
	// foreclosed IApplication, so waiting would stall forever.
	HasUndrainedEpochsBeforeBlock(ctx context.Context, appID int64, blockBound uint64) (bool, error)

	// HasUnpublishedEpochBeforeBlock keeps the machine available while a healthy,
	// enabled, foreclosed Authority/Quorum application still has a CLOSED epoch
	// before both blockBound and foreclosure, within its publication cutoff.
	// CLOSED -> INPUTS_PROCESSED needs the resident machine's accepted-state proof
	// even when no NONE inputs remain. INPUTS_PROCESSED validation uses stored
	// data and does not keep the machine. The caller separately gates on the
	// input scanner reaching foreclosure and on HasUndrainedEpochsBeforeBlock.
	HasUnpublishedEpochBeforeBlock(ctx context.Context, appID int64, blockBound uint64) (bool, error)

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
	// that overlap the foreclosure block as CLAIM_FORECLOSED. Under ET/IOR it
	// also closes the suffix at or beyond the typed epoch cutoff: that suffix
	// has no valid local claim path. Earlier epochs retain validation and
	// reconciliation; CLAIM_ACCEPTED is preserved. FAILED/DC and disabled apps
	// cannot authorize this terminalization.
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

const MaxOutputRangeRows uint64 = 65_536

// EpochOutputRange describes one epoch's accepted outputs in one database snapshot.
type EpochOutputRange struct {
	Count uint64
	First uint64
	Last  uint64
}

// OutputScanFunc handles one output. rawData is valid only during the call;
// fn must not keep it after returning.
type OutputScanFunc func(index uint64, rawData []byte) error

type OutputRepository interface {
	GetEpochOutputRange(ctx context.Context, appID int64, epochIndex uint64) (EpochOutputRange, error)
	// ScanOutputs streams both inclusive bounds in ascending index order.
	// The range must contain between 1 and MaxOutputRangeRows indexes.
	ScanOutputs(ctx context.Context, appID int64, first, last uint64, fn OutputScanFunc) error
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

const MaxStateHashPageSize uint64 = 100_000

type StateHashRepository interface {
	// CountEpochStateHashes returns the number of stored rows in one epoch.
	CountEpochStateHashes(ctx context.Context, appID int64, epochIndex uint64) (uint64, error)
	// ListEpochStateHashes returns rows in ascending index order, exclusive of
	// afterIndex. A nil cursor starts at the first row. The limit controls the
	// caller's memory and must be between 1 and MaxStateHashPageSize.
	// Processed epochs have immutable rows and can be read in separate pages.
	ListEpochStateHashes(ctx context.Context, appID int64, epochIndex uint64, afterIndex *uint64, limit uint64) ([]EpochStateHash, error)
}

type TournamentRepository interface {
	// CreateTournament preserves immutable facts for an existing
	// (application_id, epoch_index, address) and can refresh its current snapshot.
	// Conflicting facts and older snapshots return ErrTournamentEventConflict.
	CreateTournament(ctx context.Context, nameOrAddress string, t *Tournament) error
	GetTournament(ctx context.Context, nameOrAddress string, address string) (*Tournament, error)
	ListTournaments(ctx context.Context, nameOrAddress string, f TournamentFilter,
		p Pagination, descending bool) ([]*Tournament, uint64, error)
	// ListRecoverableRootBonds lists owned RECOVERABLE roots in ascending epoch order,
	// regardless of local claim status or join history. observedBlock must not exceed
	// the configured observation head. Each row certifies its own observation.
	// afterEpoch is exclusive (nil includes epoch zero); throughEpoch is inclusive.
	// limit must be positive and fit in a PostgreSQL signed integer. No count is read.
	ListRecoverableRootBonds(ctx context.Context, appID int64, claimer common.Address, observedBlock uint64,
		afterEpoch *uint64, throughEpoch uint64, limit uint64) ([]RootBondRecoveryCandidate, error)
}

type CommitmentRepository interface {
	CreateCommitment(ctx context.Context, nameOrAddress string, c *Commitment) error
	GetCommitment(ctx context.Context, nameOrAddress string, epochIndex uint64, tournamentAddress string, commitmentHex string) (*Commitment, error)
	ListCommitments(ctx context.Context, nameOrAddress string, f CommitmentFilter, p Pagination, descending bool) ([]*Commitment, uint64, error)
}

type MatchRepository interface {
	CreateMatch(ctx context.Context, nameOrAddress string, m *Match) error
	GetMatch(ctx context.Context, nameOrAddress string, epochIndex uint64, tournamentAddress string, idHashHex string) (*Match, error)
	ListMatches(ctx context.Context, nameOrAddress string, f MatchFilter, p Pagination, descending bool) ([]*Match, uint64, error)
}

type MatchAdvancedRepository interface {
	CreateMatchAdvanced(ctx context.Context, nameOrAddress string, m *MatchAdvanced) error
	GetMatchAdvanced(ctx context.Context, nameOrAddress string, epochIndex uint64, tournamentAddress string,
		idHashHex string, txHash common.Hash, logIndex uint64) (*MatchAdvanced, error)
	ListMatchAdvances(ctx context.Context, nameOrAddress string, epochIndex uint64, tournamentAddress string,
		idHashHex string, p Pagination, descending bool) ([]*MatchAdvanced, uint64, error)
}

type BondEventRepository interface {
	GetBondEvent(ctx context.Context, nameOrAddress string, txHash common.Hash, logIndex uint64) (*BondEvent, error)
	ListBondEvents(ctx context.Context, nameOrAddress string, f BondEventFilter, p Pagination, descending bool) ([]*BondEvent, uint64, error)
}

// TournamentEventBatch contains one tournament projection and the events from
// a completed scan window. Batches are ordered with parents before children:
// a child tournament references a match written in its parent's batch.
type TournamentEventBatch struct {
	Tournament    *Tournament
	Commitments   []*Commitment
	Matches       []*Match
	MatchAdvances []*MatchAdvanced
	BondEvents    []*BondEvent
}

type BulkOperationsRepository interface {
	StoreAdvanceResult(ctx context.Context, appID int64, result *AdvanceResult) error
	StoreClaimAndProofs(ctx context.Context, epoch *Epoch, write ClaimProofWriteFunc) error
}

type NodeConfigReader interface {
	LoadNodeConfigRaw(ctx context.Context, key string) (rawJSON []byte, createdAt, updatedAt time.Time, err error)
}

// ClaimRoots is the result of one coherent local prefix read. Ready requires
// input coverage and validation of every stored predecessor, including CF rows.
type ClaimRoots struct {
	Ready       bool
	MachineHash common.Hash
	OutputsRoot common.Hash
	ReasonClass string // Stable warning class; numeric details belong in Reason.
	Reason      string
}

// TODO: migrate ClaimRow -> Application + Epoch and use the other interfaces
type ClaimerRepository interface {
	ReadClaimRoots(ctx context.Context, appID int64, covered uint64) (ClaimRoots, error)
	SelectClaimsToSubmitPage(ctx context.Context, cursor ClaimCursor, limit uint64) (ClaimPage, error)
	SelectClaimsToStagePage(ctx context.Context, cursor ClaimCursor, limit uint64) (ClaimPage, error)
	SelectClaimsToAcceptPage(ctx context.Context, cursor ClaimCursor, limit uint64) (ClaimPage, error)

	UpdateEpochWithSubmittedClaim(
		ctx context.Context,
		applicationID int64,
		index uint64,
		transactionHash common.Hash,
	) error
	// RefreshClaimObservation replaces only audit metadata for the same still-open
	// claim. The caller verifies matching events at the selected observation view.
	// A nil hash records the absence of canonical own-vote evidence; it does not
	// prevent recording the staging clock of a valid majority claim.
	RefreshClaimObservation(ctx context.Context, expected *Epoch, transactionHash *common.Hash, stagedAtBlock *uint64) error
	UpdateEpochToStaged(
		ctx context.Context,
		applicationID int64,
		index uint64,
		stagedAtBlock uint64,
	) error
	// UpdateEpochStagingBlock refreshes only a still-staged PRT epoch whose saved
	// staging block matches the caller's previous observation. The caller must
	// verify that the claim content is unchanged at the selected chain view.
	UpdateEpochStagingBlock(ctx context.Context, applicationID int64, index, expectedOldBlock, newBlock uint64) error
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
	// RejectEpochAndSetApplicationDiverged atomically attempts to mark an epoch
	// as CLAIM_REJECTED and the application as DIVERGED. The returned result
	// reports which conditional writes applied, so callers can mirror only the
	// durable transitions. An existing DIVERGED/CORRUPTED diagnosis remains
	// unchanged; execution/root stops may escalate while retaining their cutoff.
	// An epoch outside CLAIM_COMPUTED/CLAIM_SUBMITTED is
	// likewise left unchanged.
	RejectEpochAndSetApplicationDiverged(
		ctx context.Context,
		applicationID int64,
		index uint64,
		reason string,
	) (RejectEpochAndDivergeResult, error)
}

// RejectEpochAndDivergeResult describes the two conditional writes performed
// by RejectEpochAndSetApplicationDiverged when the returned error is nil.
type RejectEpochAndDivergeResult struct {
	EpochRejected       bool
	ApplicationDiverged bool
}

type Repository interface {
	DeliveryRepository
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
	BondEventRepository
	BulkOperationsRepository
	NodeConfigReader
	ClaimerRepository
	WithdrawalRepository
	Close()
}

// LoadNodeConfig returns a non-nil record when err is nil. A missing JSON payload
// is an error, even if the repository reports a successful read.
func LoadNodeConfig[T any](
	ctx context.Context,
	repo NodeConfigReader,
	key string,
) (*NodeConfig[T], error) {
	raw, createdAt, updatedAt, err := repo.LoadNodeConfigRaw(ctx, key)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, fmt.Errorf("node config %q has no JSON value", key)
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
