// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"

	"github.com/ethereum/go-ethereum/common"
)

type iclaimerRepository interface {
	ListApplicationsByID(context.Context, repository.ApplicationFilter, int64, int64, uint64) ([]*model.Application, error)
	SelectClaimsToSubmitPage(context.Context, repository.ClaimCursor, uint64) (repository.ClaimPage, error)
	SelectClaimsToStagePage(context.Context, repository.ClaimCursor, uint64) (repository.ClaimPage, error)
	SelectClaimsToAcceptPage(context.Context, repository.ClaimCursor, uint64) (repository.ClaimPage, error)

	UpdateEpochWithSubmittedClaim(
		ctx context.Context,
		applicationID int64,
		index uint64,
		transactionHash common.Hash,
	) error
	RefreshClaimObservation(
		ctx context.Context, expected *model.Epoch, transactionHash *common.Hash, stagedAtBlock *uint64,
	) error

	UpdateEpochToStaged(
		ctx context.Context,
		applicationID int64,
		index uint64,
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
}
