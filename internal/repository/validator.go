// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"fmt"
	"time"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/jackc/pgx/v5"
)

const DefaultServiceTimeout = 5 * time.Minute

func (pg *database) GetLastProcessedBlock(
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

func (pg *database) GetAllOutputsFromProcessedInputs(
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

func (pg *database) getAllOutputsFromProcessedInputs(
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

func (pg *database) FinishEpoch(
	ctx context.Context,
	claim *Claim,
	outputs []Output,
) error {
	query1 := `
	INSERT INTO claim
		(index,
		output_merkle_root_hash,
		status,
		application_address)
	VALUES
		(@index,
		@outputMerkleRootHash,
		@status,
		@appAddress)`

	query2 := `
	UPDATE output
	SET
		output_hashes_siblings=@outputHashesSiblings
	WHERE
		index=@index`

	args := pgx.NamedArgs{
		"index":                claim.Index,
		"status":               ClaimStatusPending,
		"outputMerkleRootHash": claim.OutputMerkleRootHash,
		"appAddress":           claim.AppAddress,
	}

	tx, err := pg.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("unable to finish epoch: %w\n", err)
	}
	_, err = tx.Exec(ctx, query1, args)
	if err != nil {
		return fmt.Errorf("unable to finish epoch: %w\n", err)
	}

	for _, output := range outputs {
		outputArgs := pgx.NamedArgs{
			"outputHashesSiblings": output.OutputHashesSiblings,
			"index":                output.Index,
		}
		_, err = tx.Exec(ctx, query2, outputArgs)
		if err != nil {
			return fmt.Errorf("unable to finish epoch: %w\n", err)
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("unable to finish epoch: %w\n", err)
	}

	return nil
}
