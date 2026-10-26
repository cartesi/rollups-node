// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Package prtprotection reports the remaining opportunity to protect sealed epochs.
package prtprotection

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"slices"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
)

// Repository supplies sealed roots and their stored clock metadata.
type Repository interface {
	ListEpochs(context.Context, string, repository.EpochFilter, repository.Pagination, bool) ([]*model.Epoch, uint64, error)
	GetTournament(context.Context, string, string) (*model.Tournament, error)
}

// Clock retains the identity and provenance of one protection deadline.
// A nil ClosingBlock means that numerical clock evidence is unavailable.
type Clock struct {
	EpochIndex   uint64
	Root         common.Address
	ClosingBlock *uint64
	Source       string
	Err          error
}

// ClosingBlock derives the exclusive end of the root's join window.
func ClosingBlock(start, allowance uint64) (uint64, error) {
	if allowance > math.MaxUint64-start {
		return 0, errors.New("root protection deadline overflows uint64")
	}
	return start + allowance, nil
}

// StoredClocks reads each open obligation without discarding siblings on error.
// Stored metadata does not establish current chain canonicality or freshness.
func StoredClocks(ctx context.Context, repo Repository, app *model.Application, firstEpoch uint64) ([]Clock, error) {
	statuses := model.NonTerminalEpochStatuses()
	epochs, _, err := repo.ListEpochs(ctx, app.IApplicationAddress.Hex(), repository.EpochFilter{
		HasTournament: new(true), Status: statuses,
	}, repository.Pagination{}, false)
	if err != nil {
		return nil, err
	}
	clocks := make([]Clock, 0, len(epochs))
	for _, epoch := range epochs {
		if epoch.Index < firstEpoch || epoch.TournamentAddress == nil || epoch.ClaimTransactionHash != nil ||
			!slices.Contains(statuses, epoch.Status) {
			continue
		}
		clock := Clock{EpochIndex: epoch.Index, Root: *epoch.TournamentAddress, Source: "stored root descriptor"}
		root, readErr := repo.GetTournament(ctx, app.IApplicationAddress.Hex(), clock.Root.Hex())
		switch {
		case readErr != nil:
			clock.Err = readErr
		case root == nil:
			clock.Err = errors.New("root descriptor is not stored")
		case epoch.ApplicationID != app.ID || root.ApplicationID != app.ID || root.EpochIndex != epoch.Index ||
			root.Address != clock.Root || root.Level != 0 || root.StartInstant != epoch.LastBlock:
			clock.Err = errors.New("stored root descriptor does not match the sealed epoch")
		default:
			closeBlock, addErr := ClosingBlock(root.StartInstant, root.Allowance)
			clock.Err = addErr
			if addErr == nil {
				clock.ClosingBlock = new(closeBlock)
			}
		}
		clocks = append(clocks, clock)
	}
	return clocks, nil
}

// ReportClock reports an automatic stop for one identified sealed root.
func ReportClock(logger *slog.Logger, app *model.Application, clock Clock, cause error) {
	attrs := []any{"application", app.Name, "address", app.IApplicationAddress, "epoch_index", clock.EpochIndex,
		"root_tournament", clock.Root, "cause", cause, "clock_source", clock.Source, "clock_available", clock.ClosingBlock != nil}
	if clock.ClosingBlock != nil {
		attrs = append(attrs, "join_close_block", *clock.ClosingBlock)
	}
	if clock.Err != nil {
		attrs = append(attrs, "clock_error", clock.Err)
	}
	logger.Error("PRT protection stopped", attrs...)
}

// ReportUnavailable reports unavailable discovery without inventing root identity.
func ReportUnavailable(logger *slog.Logger, app *model.Application, cause error) {
	logger.Error("PRT protection deadline discovery unavailable", "application", app.Name, "address", app.IApplicationAddress,
		"cause", cause, "clock_available", false, "clock_source", "unavailable")
}

// ReportStop reports the affected open obligations without changing service state.
func ReportStop(ctx context.Context, logger *slog.Logger, repo Repository, app *model.Application, firstEpoch uint64, cause error) {
	if !app.IsDaveConsensus() {
		return
	}
	clocks, err := StoredClocks(ctx, repo, app, firstEpoch)
	if err != nil {
		ReportUnavailable(logger, app, errors.Join(cause, err))
	}
	for _, clock := range clocks {
		ReportClock(logger, app, clock, cause)
	}
}

// ReportGlobalUnavailable reports a service stop without delaying it for discovery.
func ReportGlobalUnavailable(logger *slog.Logger, cause error) {
	logger.Error("PRT global protection deadline inventory unavailable",
		"affected_applications", "all enabled PRT applications", "cause", cause,
		"clock_available", false, "clock_source", "unavailable")
}
