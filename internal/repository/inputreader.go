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

func (pg *database) InsertInputsAndUpdateLastProcessedBlock(
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
		application_address)
	VALUES
		(@index,
		@status,
		@rawData,
		@blockNumber,
		@appAddress)`

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
		}
		_, err = tx.Exec(ctx, query, inputArgs)
		if err != nil {
			return fmt.Errorf("%w: %w", errInsertInputs, err)
		}
	}

	_, err = tx.Exec(ctx, query2, args)
	if err != nil {
		return fmt.Errorf("%w: %w", errInsertInputs, err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", errInsertInputs, err)
	}

	return nil
}

func (pg *database) GetAllRunningApplications(
	ctx context.Context,
) ([]Application, error) {
	var (
		id                 uint64
		contractAddress    Address
		templateHash       Hash
		snapshotUri        string
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
		snapshot_uri,
		last_processed_block,
		epoch_length,
		status
	FROM
		application
	WHERE
		status='RUNNING'
	ORDER BY
		id asc`

	rows, err := pg.db.Query(ctx, query)
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
