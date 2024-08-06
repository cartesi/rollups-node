// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// GetOutputs returns outputs produced by inputs sent to the application
// between start and end blocks, inclusive. Outputs are in ascending order
// by index.
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

// GetProcessedEpochs returns epochs from the application which had all
// its inputs processed. Epochs are in ascending order by index.
func (pg *Database) GetProcessedEpochs(ctx context.Context, application Address) ([]*Epoch, error) {

	query := `
	SELECT
		id,
    	application_address,
        index,
        first_block,
		last_block,
		claim_hash,
		transaction_hash,
		status,
	FROM
		epoch
	WHERE
		application_address=@appAddress and status=@status
	ORDER BY
		index asc`

	args := pgx.NamedArgs{
		"application_address": application,
		"status":              EpochStatusProcessedAllInputs,
	}

	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("Query failed: %v\n", err)
	}

	var id, index, firstBlock, lastBlock uint64
	var claimHash, transactionHash Hash
	var status string
	var appAddress Address

	rowCount := 0
	var results []*Epoch
	_, err = pgx.ForEachRow(rows,
		[]any{&id,
			&appAddress,
			&index,
			&firstBlock,
			&lastBlock,
			&claimHash,
			&transactionHash,
			&status},
		func() error {
			rowCount++
			if status != string(EpochStatusProcessedAllInputs) {
				epoch := &Epoch{
					Id:              id,
					Index:           index,
					AppAddress:      appAddress,
					FirstBlock:      firstBlock,
					LastBlock:       lastBlock,
					ClaimHash:       &claimHash,
					TransactionHash: &transactionHash,
				}
				results = append(results, epoch)
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

// GetPreviousEpoch returns the epoch that ended one block before the start
// of the current epoch
func (pg *Database) GetPreviousEpoch(ctx context.Context, currentEpoch *Epoch) (*Epoch, error) {

	if currentEpoch == nil {
		return nil, fmt.Errorf("currentEpoch cannot be nil")
	}

	if currentEpoch.FirstBlock == 0 {
		return nil, nil
	}

	var (
		id, index, firstBlock, lastBlock uint64
		applicationAddress               Address
		claimHash, transactionHash       *Hash
		status                           EpochStatus
	)

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
		application_address=@appAddress and last_block=@lastBlock
	ORDER BY
		index desc`

	args := pgx.NamedArgs{
		"appAddress": currentEpoch.AppAddress,
		"lastBlock":  currentEpoch.FirstBlock - 1,
	}

	err := pg.db.QueryRow(ctx, query, args).Scan(
		&id,
		&applicationAddress,
		&index,
		&firstBlock,
		&lastBlock,
		&transactionHash,
		&claimHash,
		&status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("GetPreviousEpoch returned no rows", "service", "repository")
			return nil, nil
		}
		return nil, fmt.Errorf("GetPreviousEpoch QueryRow failed: %w\n", err)
	}

	epoch := Epoch{
		Id:              id,
		Index:           index,
		FirstBlock:      firstBlock,
		LastBlock:       lastBlock,
		TransactionHash: transactionHash,
		ClaimHash:       claimHash,
		Status:          status,
		AppAddress:      applicationAddress,
	}

	return &epoch, nil

}

// GetLastInputOutputHash returns the outputs Merkle tree hash calculated
// by the Cartesi Machine after it processed the last input in the provided
// epoch.
func (pg *Database) GetLastInputOutputHash(ctx context.Context, epoch *Epoch) (*Hash, error) {

	//Get Epoch from Database
	epoch, err := pg.GetEpoch(ctx, epoch.Index, epoch.AppAddress)
	if err != nil {
		return nil, err
	}

	//Check Epoch Status
	switch epoch.Status {
	case EpochStatusReceivingInputs:
		fallthrough
	case EpochStatusReceivedLastInput:
		return nil, fmt.Errorf("Epoch '%d' still being processed", epoch.Index)
	case EpochStatusProcessedAllInputs:
		fallthrough
	case EpochStatusCalculatedClaim:
		fallthrough
	case EpochStatusSubmittedClaim:
		fallthrough
	case EpochStatusAcceptedClaim:
		fallthrough
	case EpochStatusRejectedClaim:
		fallthrough
	default:
		break
	}

	//Get epoch last input
	query := `
	SELECT
		outputs_hash
	FROM input
	WHERE
		epoch_id = @epochId
	ORDER By
		index DESC
	LIMIT 1

	`
	var outputHash Hash

	args := pgx.NamedArgs{
		"epochId": epoch.Id,
	}

	err = pg.db.QueryRow(ctx, query, args).Scan(
		&outputHash,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("No inputs", "service", "repository", "epoch", epoch.Index)
			return nil, nil
		}
		return nil, fmt.Errorf("GetApplication QueryRow failed: %w\n", err)
	}

	return &outputHash, nil

}
