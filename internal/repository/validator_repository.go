// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/jackc/pgx/v5"
)

func (pg *database) InsertFirstEpochTransaction(
	ctx context.Context,
	epoch *Epoch,
) error {
	query := `
	BEGIN;
	INSERT INTO epochs
		(start_block,
		end_block)
	SELECT
		@startBlock,
		@endBlock
	WHERE NOT EXISTS (SELECT * FROM epochs);
	UPDATE node_state SET current_epoch = @startBlock;
	COMMIT;`
	args := pgx.NamedArgs{
		"startBlock": epoch.StartBlock,
		"endBlock":   epoch.EndBlock,
	}
	_, err := pg.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("unable to insert row: %w\n", err)
	}

	return nil
}

func (pg *database) FinishEmptyEpochTransaction(
	ctx context.Context,
	epoch *Epoch,
) error {
	query := `
	BEGIN;
	INSERT INTO epochs
		(start_block,
		end_block)
	VALUES
		(@startBlock,
		@endBlock);
	UPDATE node_state SET current_epoch = @startBlock;
	COMMIT;`
	args := pgx.NamedArgs{
		"startBlock": epoch.StartBlock,
		"endBlock":   epoch.EndBlock,
	}
	_, err := pg.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("unable to insert row: %w\n", err)
	}

	return nil
}

func (pg *database) GetMostRecentBlock(
	ctx context.Context,
) (*uint64, error) {
	var block uint64

	query := `
	SELECT
		most_recently_finalized_block
	FROM
		node_state`

	err := pg.db.QueryRow(ctx, query).Scan(&block)
	if err != nil {
		return nil, fmt.Errorf("QueryRow failed: %v\n", err)
	}

	return &block, nil
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
		i.start_block BETWEEN $1 and $2`

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	to := time.NewTimer(*timeout)
	defer to.Stop()
	for {
		select {
		case <-to.C:
			return nil, fmt.Errorf("the query timed out") // timeout
		case <-ticker.C:
			rows, err := pg.db.Query(ctx, query, startBlock, endBlock)
			if err != nil {
				return nil, fmt.Errorf("QueryRow failed: %v\n", err)
			}

			var input_index, index uint64
			var blob []byte
			var status string
			var results []Output

			count := 0

			pgx.ForEachRow(rows, []any{&input_index, &index, &blob, &status}, func() error {
				count++
				if status != "UNPROCESSED" {
					output := Output{
						InputIndex: input_index,
						Index:      index,
						Blob:       blob,
					}
					results = append(results, output)
				}
				return nil
			})

			if len(results) == count {
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
	BEGIN;
	INSERT INTO epochs
		(start_block,
		end_block)
	VALUES
		($1,
		$2);
	UPDATE node_state SET current_epoch = $3;
	INSERT INTO claims
		(id,
		epoch,
		first_input_index,
		last_input_index,
		epoch_hash,
		application_address)
	VALUES
		($4,
		$5,
		$6,
		$7,
		$8,
		$9);
`

	valueStrings := make([]string, 0, len(proofs))
	valueArgs := make([]interface{}, 0, 9+len(proofs)*9)

	valueArgs = append(valueArgs, nextEpoch.StartBlock)
	valueArgs = append(valueArgs, nextEpoch.EndBlock)
	valueArgs = append(valueArgs, nextEpoch.StartBlock)
	valueArgs = append(valueArgs, claim.Id)
	valueArgs = append(valueArgs, claim.Epoch)
	valueArgs = append(valueArgs, claim.InputRange.First)
	valueArgs = append(valueArgs, claim.InputRange.Last)
	valueArgs = append(valueArgs, claim.EpochHash)
	valueArgs = append(valueArgs, claim.AppAddress)

	i := 1
	for _, proof := range proofs {
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)", i*9+1, i*9+2, i*9+3, i*9+4, i*9+5, i*9+6, i*9+7, i*9+8, i*9+9))
		valueArgs = append(valueArgs, proof.InputIndex)
		valueArgs = append(valueArgs, proof.ClaimId)
		valueArgs = append(valueArgs, proof.InputIndexWithinEpoch)
		valueArgs = append(valueArgs, proof.OutputIndexWithinInput)
		valueArgs = append(valueArgs, proof.OutputHashesRootHash)
		valueArgs = append(valueArgs, proof.OutputsEpochRootHash)
		valueArgs = append(valueArgs, proof.MachineStateHash)
		valueArgs = append(valueArgs, proof.OutputHashInOutputHashesSiblings)
		valueArgs = append(valueArgs, proof.OutputHashesInEpochSiblings)
		i++
	}

	query2 := fmt.Sprintf(`
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
	VALUES %s;
	COMMIT;`, strings.Join(valueStrings, ","))

	query := query1 + query2

	_, err := pg.db.Exec(ctx, query, valueArgs)
	if err != nil {
		return fmt.Errorf("unable to finish epoch: %w\n", err)
	}

	return nil
}
