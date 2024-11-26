// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"fmt"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"

	"github.com/jackc/pgx/v5"
)

var (
	ErrNoUpdate = fmt.Errorf("update did not take effect")
)

type ClaimRow struct {
	EpochID              uint64
	EpochIndex           uint64
	EpochFirstBlock      uint64
	EpochLastBlock       uint64
	EpochHash            Hash
	AppContractAddress   Address
	AppIConsensusAddress Address
}

// Retrieve the computed claim of each application with the smallest index.
// The query may return either 0 or 1 entries per application.
func (pg *Database) SelectOldestComputedClaimPerApp(ctx context.Context) (
	map[Address]ClaimRow,
	error,
) {
	// NOTE(mpolitzer): DISTINCT ON is a postgres extension. To implement
	// this in SQLite there is an alternative using GROUP BY and HAVING
	// clauses instead.
	query := `
	SELECT DISTINCT ON(application_address)
		epoch.id,
		epoch.index,
		epoch.first_block,
		epoch.last_block,
		epoch.claim_hash,
		application.contract_address,
		application.iconsensus_address
	FROM
		epoch
	INNER JOIN
		application
	ON
		epoch.application_address = application.contract_address
	WHERE
		epoch.status=@status
	ORDER BY
		application_address, index ASC;
	`

	args := pgx.NamedArgs{
		"status": EpochStatusClaimComputed,
	}
	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}

	var data ClaimRow
	scans := []any{
		&data.EpochID,
		&data.EpochIndex,
		&data.EpochFirstBlock,
		&data.EpochLastBlock,
		&data.EpochHash,
		&data.AppContractAddress,
		&data.AppIConsensusAddress,
	}

	results := map[Address]ClaimRow{}
	_, err = pgx.ForEachRow(rows, scans, func() error {
		results[data.AppContractAddress] = data
		return nil
	})
	return results, err
}

// Retrieve the newest accepted claim of each application
func (pg *Database) SelectNewestAcceptedClaimPerApp(ctx context.Context) (
	map[Address]ClaimRow,
	error,
) {
	query := `
	SELECT DISTINCT ON(application_address)
		epoch.id,
		epoch.index,
		epoch.first_block,
		epoch.last_block,
		epoch.claim_hash,
		application.contract_address,
		application.iconsensus_address
	FROM
		epoch
	INNER JOIN
		application
	ON
		epoch.application_address = application.contract_address
	WHERE
		epoch.status=@status
	ORDER BY
		application_address, index DESC;
	`

	args := pgx.NamedArgs{
		"status": EpochStatusClaimAccepted,
	}
	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}

	var data ClaimRow
	scans := []any{
		&data.EpochID,
		&data.EpochIndex,
		&data.EpochFirstBlock,
		&data.EpochLastBlock,
		&data.EpochHash,
		&data.AppContractAddress,
		&data.AppIConsensusAddress,
	}

	results := map[Address]ClaimRow{}
	_, err = pgx.ForEachRow(rows, scans, func() error {
		results[data.AppContractAddress] = data
		return nil
	})
	return results, err
}

func (pg *Database) SelectClaimPairsPerApp(ctx context.Context) (
	map[Address]ClaimRow,
	map[Address]ClaimRow,
	error,
) {
	tx, err := pg.db.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Commit(ctx)

	computed, err := pg.SelectOldestComputedClaimPerApp(ctx)
	if err != nil {
		return nil, nil, err
	}

	accepted, err := pg.SelectNewestAcceptedClaimPerApp(ctx)
	if err != nil {
		return nil, nil, err
	}

	return computed, accepted, err
}

func (pg *Database) UpdateEpochWithSubmittedClaim(
	ctx context.Context,
	id uint64,
	transaction_hash common.Hash,
) error {
	query := `
	UPDATE
		epoch
	SET
		status = @status,
		transaction_hash = @transaction_hash
	WHERE
		status = @prevStatus AND epoch.id = @id`

	args := pgx.NamedArgs{
		"id":               id,
		"transaction_hash": transaction_hash,
		"status":           EpochStatusClaimSubmitted,
		"prevStatus":       EpochStatusClaimComputed,
	}
	tag, err := pg.db.Exec(ctx, query, args)

	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNoUpdate
	}
	return nil
}
