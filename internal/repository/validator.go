// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/jackc/pgx/v5"
)

const DefaultServiceTimeout = 5 * time.Minute

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

func (pg *Database) GetAllOutputsFromProcessedInputs(
	ctx context.Context,
	startBlock uint64,
	endBlock uint64,
	timeout *time.Duration,
) ([]Output, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	for {
		select {
		case <-ctxTimeout.Done():
			return nil, fmt.Errorf("GetAllOutputsFromProcessedInputs timeout") // timeout
		default:
			outputs, err := pg.getAllOutputsFromProcessedInputs(ctxTimeout, startBlock, endBlock)
			if outputs != nil {
				return outputs, nil
			}

			if err != nil {
				return nil, err
			}
		}
	}
}

func (pg *Database) getAllOutputsFromProcessedInputs(
	ctx context.Context,
	startBlock uint64,
	endBlock uint64,
) ([]Output, error) {
	query := `
	SELECT
		o.id,
		o.index,
		o.raw_data,
		o.input_id,
		i.status
	FROM
		output o
	INNER JOIN
		input i
	ON
		o.input_id=i.id
	WHERE
		i.block_number BETWEEN @startBlock and @endBlock
	ORDER BY
		o.index asc`

	args := pgx.NamedArgs{
		"startBlock": startBlock,
		"endBlock":   endBlock,
	}

	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("Query failed: %v\n", err)
	}

	var id, input_id, index uint64
	var rawData []byte
	var status string
	var results []Output

	rowCount := 0

	_, err = pgx.ForEachRow(rows, []any{&id, &index, &rawData, &input_id, &status},
		func() error {
			rowCount++
			if status != string(InputStatusNone) {
				output := Output{
					Id:      id,
					Index:   index,
					RawData: rawData,
					InputId: input_id,
				}
				results = append(results, output)
			}
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("ForEachRow failed: %w\n", err)
	}

	if len(results) == rowCount {
		return results, nil
	}

	return nil, nil
}

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
	 id=@id`

	args := pgx.NamedArgs{
		"id":        epoch.Id,
		"claimHash": epoch.ClaimHash,
		"status":    EpochStatusCalculatedClaim,
	}

	tx, err := pg.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("unable to set claim. epoch: '%d' ,error: %w\n", epoch.Index, err)
	}
	tag, err := tx.Exec(ctx, query1, args)
	if err != nil {
		return errors.Join(
			fmt.Errorf("unable to set claim. epoch '%d' ,error: %w\n", epoch.Index, err),
			tx.Rollback(ctx),
		)
	}
	if tag.RowsAffected() != 1 {
		return errors.Join(
			fmt.Errorf(
				"unable to set claim. epoch '%d' , error: no rows affected ",
				epoch.Index,
			),
			tx.Rollback(ctx),
		)
	}

	query2 := `
	UPDATE output
	SET
		output_hashes_siblings=@outputHashesSiblings
	WHERE
		id=@id`

	for _, output := range outputs {
		outputArgs := pgx.NamedArgs{
			"outputHashesSiblings": output.OutputHashesSiblings,
			"id":                   output.Id,
		}
		tag, err := tx.Exec(ctx, query2, outputArgs)
		if err != nil {
			return errors.Join(
				fmt.Errorf(
					`unable to set claim. epoch '%d'
					, error: unable to insert proof for output '%d' %w\n`,
					epoch.Index, output.Index, err),
				tx.Rollback(ctx),
			)
		}
		if tag.RowsAffected() != 1 {
			return errors.Join(
				fmt.Errorf(
					`unable to set claim. epoch '%d'
					, error: no rows affected on output '%d' update`,
					epoch.Index,
					output.Index,
				),
				tx.Rollback(ctx),
			)
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return errors.Join(
			fmt.Errorf("unable to set claim. epoch '%d', error: %w\n",
				epoch.Index,
				err),
			tx.Rollback(ctx),
		)
	}

	return nil
}
