// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/jackc/pgx/v5"
)

// GetOutputsProducedInBlockRange returns outputs produced by inputs sent to the application
// between start and end blocks, inclusive. Outputs are returned in ascending
// order by index.
func (pg *Database) GetOutputsProducedInBlockRange(
	ctx context.Context,
	application Address,
	startBlock uint64,
	endBlock uint64,
) ([]Output, error) {
	query := `
	SELECT
		o.id,
		o.index,
		o.raw_data,
		o.hash,
		o.output_hashes_siblings,
		o.input_id
	FROM
		output o
	INNER JOIN
		input i
	ON
		o.input_id=i.id
	WHERE
		i.block_number BETWEEN @startBlock AND @endBlock
	AND
		i.application_address=@appAddress
	ORDER BY
		o.index ASC
	`

	args := pgx.NamedArgs{"startBlock": startBlock, "endBlock": endBlock, "appAddress": application}
	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("GetOutputs failed: %w", err)
	}

	var (
		id, index, inputId   uint64
		rawData              []byte
		hash                 *Hash
		outputHashesSiblings []Hash
		outputs              []Output
	)
	scans := []any{&id, &index, &rawData, &hash, &outputHashesSiblings, &inputId}
	_, err = pgx.ForEachRow(rows, scans, func() error {
		output := Output{
			Id:                   id,
			Index:                index,
			RawData:              rawData,
			Hash:                 hash,
			OutputHashesSiblings: outputHashesSiblings,
			InputId:              inputId,
		}
		outputs = append(outputs, output)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("GetOutputs failed: %w", err)
	}
	return outputs, nil
}

// GetProcessedEpochs returns epochs from the application which had all
// its inputs processed. Epochs are returned in ascending order by index.
func (pg *Database) GetProcessedEpochs(ctx context.Context, application Address) ([]Epoch, error) {
	query := `
	SELECT
		id,
		application_address,
		index,
		first_block,
		last_block,
		claim_hash,
		transaction_hash,
		status
	FROM
		epoch
	WHERE
		application_address=@appAddress AND status=@status
	ORDER BY
		index ASC`

	args := pgx.NamedArgs{
		"appAddress": application,
		"status":     EpochStatusProcessedAllInputs,
	}

	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("GetProcessedEpochs failed: %w", err)
	}

	var (
		id, index, firstBlock, lastBlock uint64
		appAddress                       Address
		claimHash, transactionHash       *Hash
		status                           string
		results                          []Epoch
	)

	scans := []any{
		&id, &appAddress, &index, &firstBlock, &lastBlock, &claimHash, &transactionHash, &status,
	}
	_, err = pgx.ForEachRow(rows, scans, func() error {
		epoch := Epoch{
			Id:              id,
			Index:           index,
			AppAddress:      appAddress,
			FirstBlock:      firstBlock,
			LastBlock:       lastBlock,
			ClaimHash:       claimHash,
			TransactionHash: transactionHash,
			Status:          EpochStatus(status),
		}
		results = append(results, epoch)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("GetProcessedEpochs failed: %w", err)
	}
	return results, nil
}

// GetLastInputOutputsHash returns the outputs Merkle tree hash calculated
// by the Cartesi Machine after it processed the last input in the provided
// epoch.
func (pg *Database) GetLastInputOutputsHash(
	ctx context.Context,
	epochIndex uint64,
	appAddress Address,
) (*Hash, error) {
	//Get Epoch from Database
	epoch, err := pg.GetEpoch(ctx, epochIndex, appAddress)
	if err != nil {
		return nil, err
	}

	//Check Epoch Status
	switch epoch.Status { //nolint:exhaustive
	case EpochStatusOpen, EpochStatusClosed:
		err := fmt.Errorf(
			"epoch '%d' of app '%v' is still being processed",
			epoch.Index, epoch.AppAddress,
		)
		return nil, err
	default:
		break
	}

	//Get epoch last input
	query := `
	SELECT
		outputs_hash
	FROM
		input
	WHERE
		epoch_id = @id
	ORDER BY
		index DESC
	LIMIT 1
	`

	args := pgx.NamedArgs{"id": epoch.Id}
	var outputHash Hash

	err = pg.db.QueryRow(ctx, query, args).Scan(&outputHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn(
				"no inputs",
				"service", "repository",
				"epoch", epoch.Index,
				"app", epoch.AppAddress,
			)
			return nil, nil
		}
		return nil, fmt.Errorf("GetLastInputOutputsHash failed: %w", err)
	}
	return &outputHash, nil
}

// GetPreviousEpoch returns the epoch that ended one block before the start
// of the current epoch
func (pg *Database) GetPreviousEpoch(ctx context.Context, currentEpoch Epoch) (*Epoch, error) {
	query := `
	SELECT
		id,
		application_address,
		index,
		first_block,
		last_block,
		claim_hash,
		transaction_hash,
		status
	FROM
		epoch
	WHERE
		application_address=@appAddress AND index < @index
	ORDER BY
		index DESC
	LIMIT 1
	`

	args := pgx.NamedArgs{
		"appAddress": currentEpoch.AppAddress,
		"index":      currentEpoch.Index,
	}

	var (
		id, index, firstBlock, lastBlock uint64
		appAddress                       Address
		claimHash, transactionHash       *Hash
		status                           EpochStatus
	)

	err := pg.db.QueryRow(ctx, query, args).Scan(
		&id,
		&appAddress,
		&index,
		&firstBlock,
		&lastBlock,
		&claimHash,
		&transactionHash,
		&status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetPreviousEpoch failed: %w", err)
	}

	return &Epoch{
		Id:              id,
		Index:           index,
		FirstBlock:      firstBlock,
		LastBlock:       lastBlock,
		TransactionHash: transactionHash,
		ClaimHash:       claimHash,
		Status:          status,
		AppAddress:      appAddress,
	}, nil
}

// SetEpochClaimAndInsertProofsTransaction performs a database transaction
// containing two operations:
//
// 1. Updates an epoch, adding its claim and modifying its status.
//
// 2. Updates several outputs with their Keccak256 hash and proof.
func (pg *Database) SetEpochClaimAndInsertProofsTransaction(
	ctx context.Context,
	epoch Epoch,
	outputs []Output,
) error {
	query1 := `
	UPDATE epoch
	SET
		claim_hash=@claimHash,
		status=@status
	WHERE
		id = @id
	`

	args := pgx.NamedArgs{
		"claimHash": epoch.ClaimHash,
		"status":    EpochStatusClaimComputed,
		"id":        epoch.Id,
	}

	tx, err := pg.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("SetEpochClaimAndInsertProofsTransaction failed: %w", err)
	}
	tag, err := tx.Exec(ctx, query1, args)
	if err != nil {
		return errors.Join(
			fmt.Errorf("SetEpochClaimAndInsertProofsTransaction failed: %w", err),
			tx.Rollback(ctx),
		)
	}
	if tag.RowsAffected() != 1 {
		return errors.Join(
			fmt.Errorf("failed to update epoch %d: no rows affected", epoch.Index),
			tx.Rollback(ctx),
		)
	}

	query2 := `
	UPDATE
		output
	SET
		hash = @hash,
		output_hashes_siblings = @outputHashesSiblings
	WHERE
	id = @id
	`

	for _, output := range outputs {
		outputArgs := pgx.NamedArgs{
			"hash":                 output.Hash,
			"outputHashesSiblings": output.OutputHashesSiblings,
			"id":                   output.Id,
		}
		tag, err := tx.Exec(ctx, query2, outputArgs)
		if err != nil {
			return errors.Join(
				fmt.Errorf("failed to insert proof for output '%d'. %w", output.Index, err),
				tx.Rollback(ctx),
			)
		}
		if tag.RowsAffected() == 0 {
			return errors.Join(
				fmt.Errorf(
					"failed to insert proof for output '%d'. No rows affected",
					output.Index,
				),
				tx.Rollback(ctx),
			)
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return errors.Join(
			fmt.Errorf("SetEpochClaimAndInsertProofsTransaction failed: %w", err),
			tx.Rollback(ctx),
		)
	}
	return nil
}
