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
)

type Pagination struct {
	Limit  uint64
	Offset uint64
}

type ApplicationFilter struct {
	State            *ApplicationState
	Enabled          *bool
	Health           *ApplicationHealth
	Active           *bool // shorthand for enabled=true, health=RUNNING, deleted_at IS NULL
	NotDeleted       *bool // filter deleted_at IS NULL (defaults to true)
	DataAvailability *DataAvailabilitySelector
	ConsensusType    *Consensus
}

type EpochFilter struct {
	Status      []EpochStatus
	BeforeBlock *uint64
}

type InputFilter struct {
	EpochIndex *uint64
	Status     *InputCompletionStatus
	NotStatus  *InputCompletionStatus
	Sender     *common.Address
}

type Range struct {
	Start uint64
	End   uint64
}

type OutputFilter struct {
	EpochIndex     *uint64
	InputIndex     *uint64
	BlockRange     *Range
	OutputType     *[]byte
	VoucherAddress *common.Address
}

type ReportFilter struct {
	EpochIndex *uint64
	InputIndex *uint64
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

type ApplicationRepository interface {
	CreateApplication(ctx context.Context, app *Application, withExecutionParameters bool) (int64, error)
	GetApplication(ctx context.Context, nameOrAddress string) (*Application, error)
	GetProcessedInputCount(ctx context.Context, nameOrAddress string) (uint64, error)
	UpdateApplication(ctx context.Context, app *Application) error
	UpdateApplicationState(ctx context.Context, appID int64, state ApplicationState, reason *string) error
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
}

type InputRepository interface {
	GetInput(ctx context.Context, nameOrAddress string, inputIndex uint64) (*Input, error)
	GetInputByTxReference(ctx context.Context, nameOrAddress string, ref *common.Hash) (*Input, error)
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
	GetLastOutputBeforeBlock(ctx context.Context, nameOrAddress string, block uint64) (*Output, error)
	GetNumberOfExecutedOutputs(ctx context.Context, nameOrAddress string) (uint64, error)
}

type ReportRepository interface {
	GetReport(ctx context.Context, nameOrAddress string, reportIndex uint64) (*Report, error)
	ListReports(ctx context.Context, nameOrAddress string, f ReportFilter, p Pagination, descending bool) ([]*Report, uint64, error)
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
	SelectSubmittedClaimPairsPerApp(ctx context.Context) (
		map[int64]*Epoch,
		map[int64]*Epoch,
		map[int64]*Application,
		error,
	)
	SelectAcceptedClaimPairsPerApp(ctx context.Context) (
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
	UpdateEpochWithAcceptedClaim(
		ctx context.Context,
		applicationID int64,
		index uint64,
	) error
}

type Repository interface {
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
