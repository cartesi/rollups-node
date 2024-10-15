// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"errors"
	"fmt"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/jackc/pgx/v5"
)

var (
	errInsertInputs           = errors.New("unable to insert inputs")
	errUpdateEpochs           = errors.New("unable to update epochs status")
	errGetEpochWithOpenClaims = errors.New("failed to get epochs with open claims")
	errGetAllApplications     = errors.New("failed to get Applications")
)

// This method should be called at the end of EVMReader read input cycle
// In a single transaction it updates or inserts epochs, insert inputs related to each epoch
// and also updates the last processed block
func (pg *Database) StoreEpochAndInputsTransaction(
	ctx context.Context,
	epochInputsMap map[*Epoch][]Input,
	blockNumber uint64,
	contractAddress Address,
) (epochIndexIdMap map[uint64]uint64, epochIndexInputIdsMap map[uint64][]uint64, _ error) {

	insertEpochQuery := `
	INSERT INTO epoch
		(application_address,
		index,
		first_block,
		last_block,
		status)
	VALUES
		(@appAddress,
		@index,
		@firstBlock,
		@lastBlock,
		@status)
	ON CONFLICT (index,application_address)
	DO UPDATE
		set status=@status
	RETURNING
		id
	`

	insertInputQuery := `
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
		id`

	updateLastBlockQuery := `
	UPDATE application
	SET
		last_processed_block = @blockNumber
	WHERE
		contract_address=@contractAddress`

	tx, err := pg.db.Begin(ctx)
	if err != nil {
		return nil, nil, errors.Join(errInsertInputs, err)
	}

	// structures to hold the ids
	epochIndexIdMap = make(map[uint64]uint64)
	epochIndexInputIdsMap = make(map[uint64][]uint64)

	for epoch, inputs := range epochInputsMap {

		// try to insert epoch
		// Insert epoch
		var epochId uint64
		insertEpochArgs := pgx.NamedArgs{
			"appAddress": epoch.AppAddress,
			"index":      epoch.Index,
			"firstBlock": epoch.FirstBlock,
			"lastBlock":  epoch.LastBlock,
			"status":     epoch.Status,
			"id":         epoch.Id,
		}
		err := tx.QueryRow(ctx, insertEpochQuery, insertEpochArgs).Scan(&epochId)

		if err != nil {
			return nil, nil, errors.Join(errInsertInputs, err, tx.Rollback(ctx))
		}

		epochIndexIdMap[epoch.Index] = epochId

		var inputId uint64

		// Insert inputs
		for _, input := range inputs {
			inputArgs := pgx.NamedArgs{
				"index":       input.Index,
				"status":      input.CompletionStatus,
				"rawData":     input.RawData,
				"blockNumber": input.BlockNumber,
				"appAddress":  input.AppAddress,
				"epochId":     epochId,
			}
			err = tx.QueryRow(ctx, insertInputQuery, inputArgs).Scan(&inputId)
			if err != nil {
				return nil, nil, errors.Join(errInsertInputs, err, tx.Rollback(ctx))
			}
			epochIndexInputIdsMap[epoch.Index] = append(epochIndexInputIdsMap[epoch.Index], inputId)
		}
	}

	// Update last processed block
	updateLastBlockArgs := pgx.NamedArgs{
		"blockNumber":     blockNumber,
		"contractAddress": contractAddress,
	}

	_, err = tx.Exec(ctx, updateLastBlockQuery, updateLastBlockArgs)
	if err != nil {
		return nil, nil, errors.Join(errInsertInputs, err, tx.Rollback(ctx))
	}

	// Commit transaction
	err = tx.Commit(ctx)
	if err != nil {
		return nil, nil, errors.Join(errInsertInputs, err, tx.Rollback(ctx))
	}

	return epochIndexIdMap, epochIndexInputIdsMap, nil
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
		id                   uint64
		contractAddress      Address
		templateHash         Hash
		lastProcessedBlock   uint64
		lastClaimCheckBlock  uint64
		lastOutputCheckBlock uint64
		status               ApplicationStatus
		iConsensusAddress    Address
		results              []Application
	)

	query := `
	SELECT
		id,
		contract_address,
		template_hash,
		last_processed_block,
		last_claim_check_block,
		last_output_check_block,
		status,
		iconsensus_address
	FROM
		application`

	var args []any
	if criteria != nil {
		query = query + " WHERE status=$1"
		args = append(args, string(*criteria))
	}

	rows, err := pg.db.Query(ctx, query, args...)
	if err != nil {
		return nil, errors.Join(errGetAllApplications, err)
	}

	_, err = pgx.ForEachRow(rows,
		[]any{&id, &contractAddress, &templateHash,
			&lastProcessedBlock, &lastClaimCheckBlock, &lastOutputCheckBlock,
			&status, &iConsensusAddress},
		func() error {
			app := Application{
				Id:                   id,
				ContractAddress:      contractAddress,
				TemplateHash:         templateHash,
				LastProcessedBlock:   lastProcessedBlock,
				LastClaimCheckBlock:  lastClaimCheckBlock,
				LastOutputCheckBlock: lastOutputCheckBlock,
				Status:               status,
				IConsensusAddress:    iConsensusAddress,
			}
			results = append(results, app)
			return nil
		})
	if err != nil {
		return nil, errors.Join(errGetAllApplications, err)
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
		return 0, fmt.Errorf("GetLastProcessedBlock failed: %w", err)
	}

	return block, nil
}

// GetPreviousEpochsWithOpenClaims retrieves all Epochs that have EpochStatusClaimSubmitted
// status and LastBlock less than 'block'
func (pg *Database) GetPreviousEpochsWithOpenClaims(
	ctx context.Context,
	app Address,
	block uint64,
) ([]*Epoch, error) {
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
		application_address=@appAddress AND status=@status AND last_block < @block
	ORDER BY
		index ASC`

	args := pgx.NamedArgs{
		"appAddress": app,
		"status":     EpochStatusClaimSubmitted,
		"block":      block,
	}

	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Join(errGetEpochWithOpenClaims, err)
	}

	var (
		id, index, firstBlock, lastBlock uint64
		appAddress                       Address
		claimHash, transactionHash       *Hash
		status                           string
		results                          []*Epoch
	)

	scans := []any{
		&id, &appAddress, &index, &firstBlock, &lastBlock, &claimHash, &transactionHash, &status,
	}
	_, err = pgx.ForEachRow(rows, scans, func() error {
		epoch := &Epoch{
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
		return nil, errors.Join(errGetEpochWithOpenClaims, err)
	}
	return results, nil
}

// UpdateEpochs update given Epochs status
// and given application LastClaimCheckBlockNumber on a single transaction
func (pg *Database) UpdateEpochs(
	ctx context.Context,
	app Address,
	claims []*Epoch,
	lastClaimCheckBlock uint64,
) error {

	updateEpochQuery := `
	UPDATE epoch
	SET
		status = @status
	WHERE
		id = @id`

	tx, err := pg.db.Begin(ctx)
	if err != nil {
		return errors.Join(errUpdateEpochs, err)
	}

	for _, claim := range claims {
		updateClaimArgs := pgx.NamedArgs{
			"status": claim.Status,
			"id":     claim.Id,
		}

		tag, err := tx.Exec(ctx, updateEpochQuery, updateClaimArgs)
		if err != nil {
			return errors.Join(errUpdateEpochs, err, tx.Rollback(ctx))
		}
		if tag.RowsAffected() != 1 {
			return errors.Join(errUpdateEpochs,
				fmt.Errorf("no row affected when updating claim %d", claim.Index),
				tx.Rollback(ctx))
		}
	}

	// Update last processed block
	updateLastBlockQuery := `
	UPDATE application
	SET
		last_claim_check_block = @blockNumber
	WHERE
		contract_address=@contractAddress`

	updateLastBlockArgs := pgx.NamedArgs{
		"blockNumber":     lastClaimCheckBlock,
		"contractAddress": app,
	}

	_, err = tx.Exec(ctx, updateLastBlockQuery, updateLastBlockArgs)
	if err != nil {
		return errors.Join(errUpdateEpochs, err, tx.Rollback(ctx))
	}

	// Commit transaction
	err = tx.Commit(ctx)
	if err != nil {
		return errors.Join(errUpdateEpochs, err, tx.Rollback(ctx))
	}

	return nil
}

func (pg *Database) UpdateOutputExecutionTransaction(
	ctx context.Context,
	app Address,
	executedOutputs []*Output,
	blockNumber uint64,
) error {

	var errUpdateOutputs = errors.New("unable to update outputs")

	tx, err := pg.db.Begin(ctx)
	if err != nil {
		return errors.Join(errUpdateOutputs, err)
	}

	updateOutputQuery := `
	UPDATE output
	SET
		transaction_hash = @hash
	WHERE
		id = @id
	`

	for _, output := range executedOutputs {
		updateOutputArgs := pgx.NamedArgs{
			"hash": output.TransactionHash,
			"id":   output.Id,
		}

		tag, err := tx.Exec(ctx, updateOutputQuery, updateOutputArgs)
		if err != nil {
			return errors.Join(errUpdateOutputs, err, tx.Rollback(ctx))
		}
		if tag.RowsAffected() != 1 {
			return errors.Join(errUpdateOutputs,
				fmt.Errorf("no rows affected when updating output %d from app %s",
					output.Index, app),
				tx.Rollback(ctx))
		}
	}

	// Update last processed block
	updateLastBlockQuery := `
	UPDATE application
	SET last_output_check_block = @blockNumber
	WHERE
		contract_address=@contractAddress`

	updateLastBlockArgs := pgx.NamedArgs{
		"blockNumber":     blockNumber,
		"contractAddress": app,
	}

	_, err = tx.Exec(ctx, updateLastBlockQuery, updateLastBlockArgs)
	if err != nil {
		return errors.Join(errUpdateOutputs, err, tx.Rollback(ctx))
	}

	// Commit transaction
	err = tx.Commit(ctx)
	if err != nil {
		return errors.Join(errUpdateOutputs, err, tx.Rollback(ctx))
	}

	return nil
}
