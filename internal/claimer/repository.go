// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"time"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"

	"github.com/ethereum/go-ethereum/common"
)

type iclaimerRepository interface {
	// key is model.Application.ID
	SelectClaimsToSubmitPerApp(ctx context.Context) (
		map[int64]*model.Epoch,
		map[int64]*model.Epoch,
		map[int64]*model.Application,
		error,
	)

	// key is model.Application.ID
	SelectClaimsToStagePerApp(ctx context.Context) (
		map[int64]*model.Epoch,
		map[int64]*model.Epoch,
		map[int64]*model.Application,
		error,
	)

	// key is model.Application.ID. The accepted map stores the newest accepted
	// epoch per app. The staged map stores the oldest staged epoch per app.
	SelectClaimsToAcceptPerApp(ctx context.Context) (
		map[int64]*model.Epoch,
		map[int64]*model.Epoch,
		map[int64]*model.Application,
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

	UpdateEpochWithAcceptedClaim(
		ctx context.Context,
		applicationID int64,
		index uint64,
		txHash *common.Hash,
	) error
	UpdateEpochWithForeclosedClaim(
		ctx context.Context,
		applicationID int64,
		index uint64,
	) error

	RejectEpochAndSetApplicationDiverged(
		ctx context.Context,
		applicationID int64,
		index uint64,
		reason string,
	) (repository.RejectEpochAndDivergeResult, error)

	UpdateApplicationStatus(
		ctx context.Context,
		appID int64,
		status model.ApplicationStatus,
		reason *string,
	) error

	HasUndrainedEpochsBeforeBlock(ctx context.Context, appID int64, blockBound uint64) (bool, error)
	HasUnreconciledClaimsBeforeBlock(ctx context.Context, appID int64, blockBound uint64) (bool, error)
	ForecloseUnacceptedEpochsAtOrAfterBlock(ctx context.Context, appID int64, blockBound uint64) (int64, error)

	// ListApplications is used to find enabled, foreclosed Authority/Quorum
	// apps that no longer have pending claim work. processForeclosedApps still
	// needs those apps so operators can see drain progress.
	ListApplications(
		ctx context.Context,
		f repository.ApplicationFilter,
		p repository.Pagination,
		descending bool,
	) ([]*model.Application, uint64, error)

	SaveNodeConfigRaw(ctx context.Context, key string, rawJSON []byte) error
	LoadNodeConfigRaw(ctx context.Context, key string) (rawJSON []byte, createdAt, updatedAt time.Time, err error)
}
