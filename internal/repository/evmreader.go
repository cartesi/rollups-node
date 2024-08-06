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
) ([]uint64, error) {
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
		@epochId)
	RETURNING
		id
	`

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
		return nil, errors.Join(errInsertInputs, err)
	}

	var (
		id  uint64
		ids []uint64
	)
	for _, input := range inputs {
		inputArgs := pgx.NamedArgs{
			"index":       input.Index,
			"status":      input.CompletionStatus,
			"rawData":     input.RawData,
			"blockNumber": input.BlockNumber,
			"appAddress":  input.AppAddress,
			"epochId":     input.EpochId,
		}
		err = tx.QueryRow(ctx, query, inputArgs).Scan(&id)
		if err != nil {
			return nil, errors.Join(errInsertInputs, err, tx.Rollback(ctx))
		}
		ids = append(ids, id)
	}

	_, err = tx.Exec(ctx, query2, args)
	if err != nil {
		return nil, errors.Join(errInsertInputs, err, tx.Rollback(ctx))
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, errors.Join(errInsertInputs, err, tx.Rollback(ctx))
	}

	return ids, nil
}

// GetAllRunningApplications returns a slice with the applications being
// actively handled by the node.
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
		status             ApplicationStatus
		results            []Application
	)

	query := `
	SELECT
		id,
		contract_address,
		template_hash,
		last_processed_block,
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
		[]any{&id, &contractAddress, &templateHash,
			&lastProcessedBlock, &status},
		func() error {
			app := Application{
				Id:                 id,
				ContractAddress:    contractAddress,
				TemplateHash:       templateHash,
				LastProcessedBlock: lastProcessedBlock,
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

func (pg *Database) GetLastProcessedBlock(
	ctx context.Context,
	appAddress Address,
) (uint64, error) {
	var block uint64

	query := `
	SELECT
		last_processed_block
	FROM
		application
	WHERE
		contract_address=@address`

	args := pgx.NamedArgs{
		"address": appAddress,
	}

	err := pg.db.QueryRow(ctx, query, args).Scan(&block)
	if err != nil {
		return 0, fmt.Errorf("QueryRow failed: %v\n", err)
	}

	return block, nil
}
