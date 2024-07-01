// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"fmt"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/jackc/pgx/v5"
)

func (pg *database) InsertInputsAndUpdateMostRecentlyFinalizedBlock(
	ctx context.Context,
	inputs []*Input,
	blockNumber uint64,
) error {
	query := `
	INSERT INTO inputs
		(index,
		status,
		blob,
		block_number,
		machine_state_hash)
	VALUES
		(@index,
		@status,
		@blob,
		@blockNumber,
		@machineStateHash)`

	query2 := `UPDATE node_state SET most_recently_finalized_block = @blockNumber;`

	args := pgx.NamedArgs{
		"blockNumber": blockNumber,
	}

	tx, err := pg.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("unable to insert inputs: %w\n", err)
	}

	for _, input := range inputs {
		inputArgs := pgx.NamedArgs{
			"index":            input.Index,
			"status":           input.CompletionStatus,
			"blob":             input.Blob,
			"blockNumber":      input.BlockNumber,
			"machineStateHash": input.MachineStateHash,
		}
		_, err = tx.Exec(ctx, query, inputArgs)
		if err != nil {
			return fmt.Errorf("unable to insert inputs: %w\n", err)
		}
	}

	_, err = tx.Exec(ctx, query2, args)
	if err != nil {
		return fmt.Errorf("unable to insert inputs: %w\n", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("unable to insert inputs: %w\n", err)
	}

	return nil
}
