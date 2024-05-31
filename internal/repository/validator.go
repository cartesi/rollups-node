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

func (pg *database) FinishEmptyEpochTransaction(
	ctx context.Context,
	nextEpoch *Epoch,
) error {
	query := `
	INSERT INTO epochs
		(start_block,
		end_block)
	VALUES
		(@startBlock,
		@endBlock);`

	query2 := `UPDATE node_state SET current_epoch = @startBlock;`

	args := pgx.NamedArgs{
		"startBlock": nextEpoch.StartBlock,
		"endBlock":   nextEpoch.EndBlock,
	}

	tx, err := pg.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("unable to finish empty epoch: %w\n", err)
	}
	_, err = tx.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("unable to finish empty epoch: %w\n", err)
	}
	_, err = tx.Exec(ctx, query2, args)
	if err != nil {
		return fmt.Errorf("unable to finish empty epoch: %w\n", err)
	}
	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("unable to finish empty epoch: %w\n", err)
	}

	return nil
}

func (pg *database) GetMostRecentlyFinalizedBlock(
	ctx context.Context,
) (uint64, error) {
	var block uint64

	query := `
	SELECT
		most_recently_finalized_block
	FROM
		node_state`

	err := pg.db.QueryRow(ctx, query).Scan(&block)
	if err != nil {
		return 0, fmt.Errorf("QueryRow failed: %v\n", err)
	}

	return block, nil
}

func (pg *database) GetCurrentEpoch(
	ctx context.Context,
) (*Epoch, error) {
	var startBlock uint64
	var endBlock uint64

	query := `
	SELECT
		e.start_block, e.end_block
	FROM
		epochs e
	INNER JOIN
		node_state ns
	ON
		e.start_block = ns.current_epoch`

	err := pg.db.QueryRow(ctx, query).Scan(&startBlock, &endBlock)
	if err != nil {
		return nil, fmt.Errorf("QueryRow failed: %v\n", err)
	}

	epoch := Epoch{
		StartBlock: startBlock,
		EndBlock:   endBlock,
	}

	return &epoch, nil
}

func (pg *database) GetAllOutputsFromProcessedInputs(
	ctx context.Context,
	startBlock uint64,
	endBlock uint64,
	timeout *time.Duration,
) ([]Output, error) {
	query := `
	SELECT
		o.input_index,
		o.index,
		o.blob,
		i.status
	FROM
		outputs o
	INNER JOIN
		inputs i
	ON
		o.input_index=i.index
	WHERE
		i.block_number BETWEEN $1 and $2`

	var to *time.Timer
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	if timeout != nil {
		to = time.NewTimer(*timeout)
	} else {
		to = time.NewTimer(DefaultServiceTimeout)
	}
	defer to.Stop()
	for {
		select {
		case <-to.C:
			return nil, fmt.Errorf("GetAllOutputsFromProcessedInputs query timed out") // timeout
		case <-ticker.C:
			rows, err := pg.db.Query(ctx, query, startBlock, endBlock)
			if err != nil {
				return nil, fmt.Errorf("Query failed: %v\n", err)
			}

			var input_index, index uint64
			var blob []byte
			var status string
			var results []Output

			rowCount := 0

			_, err = pgx.ForEachRow(rows, []any{&input_index, &index, &blob, &status},
				func() error {
					rowCount++
					if status != string(InputStatusNone) {
						output := Output{
							InputIndex: input_index,
							Index:      index,
							Blob:       blob,
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
		}
	}
}

func (pg *database) FinishEpochTransaction(
	ctx context.Context,
	nextEpoch Epoch,
	claim *Claim,
	proofs []Proof,
) error {
	query1 := `
	INSERT INTO epochs
		(start_block,
		end_block)
	VALUES
		(@startBlock,
		@endBlock)`
	query2 := `UPDATE node_state SET current_epoch = @startBlock`
	query3 := `
	INSERT INTO claims
		(id,
		epoch,
		first_input_index,
		last_input_index,
		epoch_hash,
		application_address)
	VALUES
		(@id,
		@startBlock,
		@firstInputIndex,
		@lastInputIndex,
		@epochHash,
		@appAddress)`

	query4 := `
	INSERT INTO proofs
		(input_index,
		claim_id,
		input_index_within_epoch,
		output_index_within_input,
		output_hashes_root_hash,
		output_epoch_root_hash,
		machine_state_hash,
		output_hash_in_output_hashes_siblings,
		output_hashes_in_epoch_siblings)
	VALUES
		(@inputIndex,
		@claimId,
		@inputIndexWithin,
		@outputIndexWithin,
		@outputHashes,
		@outputEpoch,
		@machineHash,
		@outputHashOutputSiblings,
		@outputHashesSiblings)`

	args := pgx.NamedArgs{
		"startBlock":      nextEpoch.StartBlock,
		"endBlock":        nextEpoch.EndBlock,
		"id":              claim.Id,
		"firstInputIndex": claim.InputRange.First,
		"lastInputIndex":  claim.InputRange.Last,
		"epochHash":       claim.EpochHash,
		"appAddress":      claim.AppAddress,
	}

	tx, err := pg.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("unable to finish epoch: %w\n", err)
	}
	_, err = tx.Exec(ctx, query1, args)
	if err != nil {
		return fmt.Errorf("unable to finish epoch: %w\n", err)
	}
	_, err = tx.Exec(ctx, query2, args)
	if err != nil {
		return fmt.Errorf("unable to finish epoch: %w\n", err)
	}
	_, err = tx.Exec(ctx, query3, args)
	if err != nil {
		return fmt.Errorf("unable to finish epoch: %w\n", err)
	}

	for _, proof := range proofs {
		proofArgs := pgx.NamedArgs{
			"inputIndex":               proof.InputIndex,
			"claimId":                  claim.Id,
			"inputIndexWithin":         proof.InputIndexWithinEpoch,
			"outputIndexWithin":        proof.OutputIndexWithinInput,
			"outputHashes":             proof.OutputHashesRootHash,
			"outputEpoch":              proof.OutputsEpochRootHash,
			"machineHash":              proof.MachineStateHash,
			"outputHashOutputSiblings": proof.OutputHashInOutputHashesSiblings,
			"outputHashesSiblings":     proof.OutputHashesInEpochSiblings,
		}
		_, err = tx.Exec(ctx, query4, proofArgs)
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
