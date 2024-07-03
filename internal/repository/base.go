// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type database struct {
	db *pgxpool.Pool
}

var (
	pgError    error
	pgInstance *database
	pgOnce     sync.Once
)

func Connect(
	ctx context.Context,
	postgresEndpoint string,
) (*database, error) {
	pgOnce.Do(func() {
		dbpool, err := pgxpool.New(ctx, postgresEndpoint)
		if err != nil {
			pgError = fmt.Errorf("unable to create connection pool: %w\n", err)
		}

		pgInstance = &database{dbpool}
	})

	return pgInstance, pgError
}

func (pg *database) Close() {
	if pg != nil {
		pg.db.Close()
	}
}

func (pg *database) SetupDatabaseState(
	ctx context.Context,
	deploymentBlock uint64,
	epochDuration uint64,
	currentEpoch uint64,
) error {
	query := `
	INSERT INTO epochs
		(start_block,
		end_block)
	SELECT
		@startBlock,
		@endBlock
	WHERE NOT EXISTS (SELECT * FROM epochs)`

	query2 := `
	INSERT INTO node_state
		(most_recently_finalized_block,
		input_box_deployment_block,
		epoch_duration,
		current_epoch)
	SELECT
		@deploymentBlock,
		@deploymentBlock,
		@epochDuration,
		@currentEpoch
	WHERE NOT EXISTS (SELECT * FROM node_state)`

	args := pgx.NamedArgs{
		"startBlock":      deploymentBlock,
		"endBlock":        deploymentBlock + (epochDuration - 1),
		"deploymentBlock": deploymentBlock,
		"epochDuration":   epochDuration,
		"currentEpoch":    currentEpoch,
	}

	tx, err := pg.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("unable to setup database state: %w\n", err)
	}
	_, err = tx.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("unable to setup database state: %w\n", err)
	}
	_, err = tx.Exec(ctx, query2, args)
	if err != nil {
		return fmt.Errorf("unable to setup database state: %w\n", err)
	}
	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("unable to setup database state: %w\n", err)
	}

	return nil
}

func (pg *database) InsertInput(
	ctx context.Context,
	input *Input,
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
	args := pgx.NamedArgs{
		"index":            input.Index,
		"status":           input.CompletionStatus,
		"blob":             input.Blob,
		"blockNumber":      input.BlockNumber,
		"machineStateHash": input.MachineStateHash,
	}
	_, err := pg.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("unable to insert row: %w\n", err)
	}

	return nil
}

func (pg *database) InsertOutput(
	ctx context.Context,
	verifiable bool,
	output *Output,
) error {
	var table string

	if verifiable {
		table = "outputs"
	} else {
		table = "reports"
	}
	query := fmt.Sprintf(`
	INSERT INTO %s
		(input_index,
		index,
		blob)
	VALUES
		(@inputIndex,
		@index,
		@blob)`, table)

	args := pgx.NamedArgs{
		"inputIndex": output.InputIndex,
		"index":      output.Index,
		"blob":       output.Blob,
	}
	_, err := pg.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("unable to insert row: %w\n", err)
	}

	return nil
}

func (pg *database) InsertClaim(
	ctx context.Context,
	claim *Claim,
	epochStartBlock uint64,
) error {
	query := `
	INSERT INTO claims
		(id,
		epoch,
		first_input_index,
		last_input_index,
		epoch_hash,
		application_address)
	VALUES
		(@id,
		@epoch,
		@firstInputIndex,
		@lastInputIndex,
		@epochHash,
		@applicationAddress)`
	args := pgx.NamedArgs{
		"id":                 claim.Id,
		"epoch":              epochStartBlock,
		"firstInputIndex":    claim.InputRange.First,
		"lastInputIndex":     claim.InputRange.Last,
		"epochHash":          claim.EpochHash,
		"applicationAddress": claim.AppAddress,
	}
	_, err := pg.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("unable to insert row: %w\n", err)
	}

	return nil
}

func (pg *database) InsertProof(
	ctx context.Context,
	proof *Proof,
	claimId uint64,
) error {
	query := `
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
		(@input_index,
		@claim_id,
		@inputIndexWithinEpoch,
		@outputIndexWithinInput,
		@outputHashesRootHash,
		@outputsEpochRootHash,
		@machineStateHash,
		@outputHashInOutputHashesSiblings,
		@outputHashesInEpochSiblings)`

	args := pgx.NamedArgs{
		"input_index":                      proof.InputIndex,
		"claim_id":                         claimId,
		"inputIndexWithinEpoch":            proof.InputIndexWithinEpoch,
		"outputIndexWithinInput":           proof.OutputIndexWithinInput,
		"outputHashesRootHash":             proof.OutputHashesRootHash,
		"outputsEpochRootHash":             proof.OutputsEpochRootHash,
		"machineStateHash":                 proof.MachineStateHash,
		"outputHashInOutputHashesSiblings": proof.OutputHashInOutputHashesSiblings,
		"outputHashesInEpochSiblings":      proof.OutputHashesInEpochSiblings,
	}
	_, err := pg.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("unable to insert row: %w\n", err)
	}

	return nil
}

func (pg *database) GetInput(
	ctx context.Context,
	index uint64,
) (*Input, error) {
	var (
		status      InputCompletionStatus
		blob        []byte
		blockNumber uint64
		machineHash Hash
	)

	query := `
	SELECT
		blob,
		status,
		block_number,
		machine_state_hash
	FROM
		inputs
	WHERE
		index=$1`

	err := pg.db.QueryRow(ctx, query, index).Scan(&blob, &status, &blockNumber, &machineHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("GetInput returned no rows", "service", "repository")
			return nil, nil
		}
		return nil, fmt.Errorf("GetInput QueryRow failed: %v\n", err)
	}

	input := Input{
		Index:            index,
		CompletionStatus: status,
		Blob:             blob,
		BlockNumber:      blockNumber,
		MachineStateHash: machineHash,
	}

	return &input, nil
}

func (pg *database) GetOutput(
	ctx context.Context,
	verifiable bool,
	inputIndex uint64,
	index uint64,
) (*Output, error) {
	var blob []byte
	var table string

	if verifiable {
		table = "outputs"
	} else {
		table = "reports"
	}
	query := fmt.Sprintf(`
	SELECT
		blob
	FROM
		%s
	WHERE
		input_index=$1 AND index=$2`, table)

	err := pg.db.QueryRow(ctx, query, inputIndex, index).Scan(&blob)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("GetOutput returned no rows", "service", "repository")
			return nil, nil
		}
		return nil, fmt.Errorf("GetOutput QueryRow failed: %v\n", err)
	}

	output := Output{
		InputIndex: inputIndex,
		Index:      index,
		Blob:       blob,
	}

	return &output, nil
}

func (pg *database) GetProof(
	ctx context.Context,
	inputIndex uint64,
	outputIndex uint64,
) (*Proof, error) {
	var (
		firstInput                       uint64
		lastInput                        uint64
		inputIndexWithinEpoch            uint64
		outputIndexWithinInput           uint64
		outputHashesRootHash             Hash
		outputsEpochRootHash             Hash
		machineStateHash                 Hash
		outputHashInOutputHashesSiblings []Hash
		outputHashesInEpochSiblings      []Hash
	)

	query := `
	SELECT
		c.first_input_index,
		c.last_input_index,
		p.input_index_within_epoch,
		p.output_index_within_input,
		p.output_hashes_root_hash,
		p.output_epoch_root_hash,
		p.machine_state_hash,
		p.output_hash_in_output_hashes_siblings,
		p.output_hashes_in_epoch_siblings
	FROM
		proofs p
	INNER JOIN
		claims c
	ON
		p.claim_id=c.id
	WHERE
		p.input_index=$1 AND p.output_index_within_input=$2`

	err := pg.db.QueryRow(ctx, query, inputIndex, outputIndex).Scan(
		&firstInput,
		&lastInput,
		&inputIndexWithinEpoch,
		&outputIndexWithinInput,
		&outputHashesRootHash,
		&outputsEpochRootHash,
		&machineStateHash,
		&outputHashInOutputHashesSiblings,
		&outputHashesInEpochSiblings,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("GetProof returned no rows", "service", "repository")
			return nil, nil
		}
		return nil, fmt.Errorf("GetProof QueryRow failed: %v\n", err)
	}

	inputRange := InputRange{
		First: firstInput,
		Last:  lastInput,
	}

	proof := Proof{
		InputIndex:                       inputIndex,
		InputRange:                       inputRange,
		InputIndexWithinEpoch:            inputIndexWithinEpoch,
		OutputIndexWithinInput:           outputIndexWithinInput,
		OutputHashesRootHash:             outputHashesRootHash,
		OutputsEpochRootHash:             outputsEpochRootHash,
		MachineStateHash:                 machineStateHash,
		OutputHashInOutputHashesSiblings: outputHashInOutputHashesSiblings,
		OutputHashesInEpochSiblings:      outputHashesInEpochSiblings,
	}

	return &proof, nil
}

func (pg *database) GetClaim(
	ctx context.Context,
	index uint64,
) (*Claim, error) {
	var (
		first      uint64
		last       uint64
		epochHash  Hash
		appAddress Address
	)

	query := `
	SELECT
		first_input_index,
		last_input_index,
		epoch_hash,
		application_address
	FROM
		claims
	WHERE
		id=$1`

	err := pg.db.QueryRow(ctx, query, index).Scan(&first, &last, &epochHash, &appAddress)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("GetClaim returned no rows", "service", "repository")
			return nil, nil
		}
		return nil, fmt.Errorf("GetClaim QueryRow failed: %v\n", err)
	}

	inputRange := InputRange{
		First: first,
		Last:  last,
	}

	claim := Claim{
		Id:         index,
		InputRange: inputRange,
		EpochHash:  epochHash,
		AppAddress: appAddress,
	}

	return &claim, nil
}
