// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"errors"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrObserverNotRegistered   = errors.New("contributing observer is not registered")
	ErrSubmitterProvenance     = errors.New("submission provenance is not usable")
	ErrTournamentIntervalStale = errors.New("tournament interval cursor changed")
)

// TournamentEventCounts counts committed structural event facts through a block.
// Mutable snapshots do not change these cumulative counts.
type TournamentEventCounts struct {
	CommitmentJoined   uint64
	MatchCreated       uint64
	MatchAdvanced      uint64
	LeafMatchSealed    uint64
	MatchDeleted       uint64
	NewInnerTournament uint64
}

type DeliveryRepository interface {
	RegisterSubmitter(ctx context.Context, key string, requested config.PersistentSubmitterConfig) (
		config.PersistentSubmitterConfig, error,
	)
	RegisterObserver(ctx context.Context, key string, requested config.PersistentChainConfig) (
		config.PersistentChainConfig, error,
	)
	CheckSubmitterProvenance(ctx context.Context, key string) error
	// StoreTournamentInterval publishes one primary tournament interval and only
	// its new descendants, in parent order, at the primary snapshot's endpoint.
	// A nil expectedAsOf requires a new root; otherwise the primary cursor must
	// equal the expectation. Existing children advance independently.
	StoreTournamentInterval(ctx context.Context, applicationID int64, expectedAsOf *uint64, batches []*TournamentEventBatch) error
	GetTournamentEventCounts(
		ctx context.Context, applicationID int64, epochIndex uint64, tournament common.Address, throughBlock uint64,
	) (TournamentEventCounts, error)
	ListApplicationsByID(
		ctx context.Context, filter ApplicationFilter, afterID, throughID int64, limit uint64,
	) ([]*model.Application, error)
	GetApplicationIDBoundary(ctx context.Context, filter ApplicationFilter) (int64, error)
	ListRecoverableRootBonds(
		ctx context.Context, applicationID int64, claimer common.Address, observedBlock uint64,
		afterEpoch *uint64, throughEpoch, limit uint64,
	) ([]RootBondRecoveryCandidate, error)
}
