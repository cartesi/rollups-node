// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package data

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgres struct {
	db *pgxpool.Pool
}

var (
	pgError    error
	pgInstance *postgres
	pgOnce     sync.Once
)

func Connect(
	ctx context.Context,
	postgres_endpoint string,
) (*postgres, error) {
	pgOnce.Do(func() {
		dbpool, err := pgxpool.New(ctx, postgres_endpoint)
		if err != nil {
			pgError = fmt.Errorf("unable to create connection pool: %w\n", err)
		}

		pgInstance = &postgres{dbpool}
	})

	return pgInstance, pgError
}

func (pg *postgres) Close() {
	pg.db.Close()
}

func (pg *postgres) InsertInput(
	ctx context.Context,
	input *Input,
) error {
	query := `
	INSERT INTO inputs 
		(index,
		status,
		blob) 
	VALUES 
		(@index,
		@status,
		@blob)`
	args := pgx.NamedArgs{
		"index":  input.Index,
		"status": input.Status,
		"blob":   input.Blob,
	}
	_, err := pg.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("unable to insert row: %w\n", err)
	}

	return nil
}

func (pg *postgres) InsertOutput(
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

func (pg *postgres) InsertProof(
	ctx context.Context,
	proof *Proof,
) error {
	query := `
	INSERT INTO proofs 
		(input_index,
		output_index,
		first_input, 
		last_input, 
		validity_input_index_within_epoch, 
		validity_output_index_within_input, 
		validity_output_hashes_root_hash,
		validity_output_epoch_root_hash,
		validity_machine_state_hash,
		validity_output_hash_in_output_hashes_siblings,
		validity_output_hashes_in_epoch_siblings) 
	VALUES 
		(@input_index,
		@output_index,
		@firstInput,
		@lastInput,
		@inputIndexWithinEpoch,
		@outputIndexWithinInput,
		@outputHashesRootHash,
		@outputsEpochRootHash,
		@machineStateHash,
		@outputHashInOutputHashesSiblings,
		@outputHashesInEpochSiblings)`

	args := pgx.NamedArgs{
		"input_index":                      proof.InputIndex,
		"output_index":                     proof.OutputIndex,
		"firstInput":                       proof.FirstInputIndex,
		"lastInput":                        proof.LastInputIndex,
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

func (pg *postgres) GetInput(
	ctx context.Context,
	index int,
) (*Input, error) {
	var status string
	var blob []byte

	query := `
	SELECT 
		blob, 
		status, 
	FROM
		inputs
	WHERE
		input=$1`

	err := pg.db.QueryRow(ctx, query, index).Scan(&blob, &status)
	if err != nil {
		return nil, fmt.Errorf("QueryRow failed: %v\n", err)
	}

	input := Input{
		index,
		status,
		blob,
	}

	return &input, nil
}

func (pg *postgres) GetOutput(
	ctx context.Context,
	verifiable bool,
	inputIndex int,
	index int,
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
		return nil, fmt.Errorf("QueryRow failed: %v\n", err)
	}

	output := Output{
		inputIndex,
		index,
		blob,
	}

	return &output, nil
}

func (pg *postgres) GetProof(
	ctx context.Context,
	inputIndex int,
	outputIndex int,
) (*Proof, error) {
	var (
		firstInput                       int
		lastInput                        int
		inputIndexWithinEpoch            int
		outputIndexWithinInput           int
		outputHashesRootHash             []byte
		outputsEpochRootHash             []byte
		machineStateHash                 []byte
		outputHashInOutputHashesSiblings [][]byte
		outputHashesInEpochSiblings      [][]byte
	)

	query := `
	SELECT 
		first_input, 
		last_input, 
		validity_input_index_within_epoch, 
		validity_output_index_within_input, 
		validity_output_hashes_root_hash,
		validity_output_epoch_root_hash,
		validity_machine_state_hash,
		validity_output_hash_in_output_hashes_siblings,
		validity_output_hashes_in_epoch_siblings
	FROM
		proofs
	WHERE
		input_index=$1 AND output_index=$2`

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
		return nil, fmt.Errorf("QueryRow failed: %v\n", err)
	}

	var outputHashOutputSiblings, outputHashEpochSiblings []hexutil.Bytes

	for _, hash := range outputHashInOutputHashesSiblings {
		outputHashOutputSiblings = append(outputHashOutputSiblings, hash)
	}

	for _, hash := range outputHashesInEpochSiblings {
		outputHashEpochSiblings = append(outputHashEpochSiblings, hash)
	}

	proof := Proof{
		inputIndex,
		outputIndex,
		firstInput,
		lastInput,
		inputIndexWithinEpoch,
		outputIndexWithinInput,
		outputHashesRootHash,
		outputsEpochRootHash,
		machineStateHash,
		outputHashOutputSiblings,
		outputHashEpochSiblings,
	}

	return &proof, nil
}
