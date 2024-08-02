// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"errors"
	"fmt"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/jackc/pgx/v5"
)

func (pg *Database) InsertInputsAndUpdateLastProcessedBlock(
	ctx context.Context,
	inputs []Input,
	blockNumber uint64,
	contractAddress Address,
) error {
	var errInsertInputs = errors.New("unable to insert inputs")

	query := `
	INSERT INTO input
		(index,
		status,
		raw_data,
		block_number,
		application_address,
		epoch_id)
	VALUES
		(@index,
		@status,
		@rawData,
		@blockNumber,
		@appAddress,
		@epochId)`

	query2 := `
	UPDATE application
	SET last_processed_block = @blockNumber
	WHERE
		contract_address=@contractAddress`

	args := pgx.NamedArgs{
		"blockNumber":     blockNumber,
		"contractAddress": contractAddress,
	}

	tx, err := pg.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", errInsertInputs, err)
	}

	for _, input := range inputs {
		inputArgs := pgx.NamedArgs{
			"index":       input.Index,
			"status":      input.CompletionStatus,
			"rawData":     input.RawData,
			"blockNumber": input.BlockNumber,
			"appAddress":  input.AppAddress,
			"epochId":     input.EpochId,
		}
		_, err = tx.Exec(ctx, query, inputArgs)
		if err != nil {
			return errors.Join(errInsertInputs, err, tx.Rollback(ctx))
		}
	}

	_, err = tx.Exec(ctx, query2, args)
	if err != nil {
		return errors.Join(errInsertInputs, err, tx.Rollback(ctx))
	}

	err = tx.Commit(ctx)
	if err != nil {
		return errors.Join(errInsertInputs, err, tx.Rollback(ctx))
	}

	return nil
}

func (pg *Database) GetAllRunningApplications(
	ctx context.Context,
) ([]Application, error) {
	criteria := ApplicationStatusRunning
	return pg.getAllApplicationsByStatus(ctx, &criteria)
}

func (pg *Database) GetAllApplications(
	ctx context.Context,
) ([]Application, error) {
	return pg.getAllApplicationsByStatus(ctx, nil)
}

func (pg *Database) getAllApplicationsByStatus(
	ctx context.Context,
	criteria *ApplicationStatus,
) ([]Application, error) {
	var (
		id                 uint64
		contractAddress    Address
		templateHash       Hash
		lastProcessedBlock uint64
		epochLength        uint64
		status             ApplicationStatus
		results            []Application
	)

	query := `
	SELECT
		id,
		contract_address,
		template_hash,
		last_processed_block,
		epoch_length,
		status
	FROM
		application
	`

	var args []any
	if criteria != nil {
		query = query + "WHERE status=$1"
		args = append(args, string(*criteria))
	}

	rows, err := pg.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("Query failed: %v\n", err)
	}

	_, err = pgx.ForEachRow(rows,
		[]any{&id, &contractAddress, &templateHash, &snapshotUri,
			&lastProcessedBlock, &epochLength, &status},
		func() error {
			app := Application{
				Id:                 id,
				ContractAddress:    contractAddress,
				TemplateHash:       templateHash,
				SnapshotURI:        snapshotUri,
				LastProcessedBlock: lastProcessedBlock,
				EpochLength:        epochLength,
				Status:             status,
			}
			results = append(results, app)
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("ForEachRow failed: %w\n", err)
	}

	return results, nil
}
