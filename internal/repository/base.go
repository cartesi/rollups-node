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

type Database struct {
	db *pgxpool.Pool
}

var ErrInsertRow = errors.New("unable to insert row")

func Connect(
	ctx context.Context,
	postgresEndpoint string,
) (*Database, error) {
	var (
		pgError    error
		pgInstance *Database
		pgOnce     sync.Once
	)

	pgOnce.Do(func() {
		dbpool, err := pgxpool.New(ctx, postgresEndpoint)
		if err != nil {
			pgError = fmt.Errorf("unable to create connection pool: %w\n", err)
		}

		pgInstance = &Database{dbpool}
	})

	return pgInstance, pgError
}

func (pg *Database) Close() {
	if pg != nil {
		pg.db.Close()
	}
}

func (pg *Database) InsertNodeConfig(
	ctx context.Context,
	config *NodePersistentConfig,
) error {
	query := `
	INSERT INTO node_config
		(default_block,
		input_box_deployment_block,
		input_box_address,
		chain_id,
		iconsensus_address)
	SELECT
		@defaultBlock,
		@deploymentBlock,
		@inputBoxAddress,
		@chainId,
		@iConsensusAddress
	WHERE NOT EXISTS (SELECT * FROM node_config)`

	args := pgx.NamedArgs{
		"defaultBlock":      config.DefaultBlock,
		"deploymentBlock":   config.InputBoxDeploymentBlock,
		"inputBoxAddress":   config.InputBoxAddress,
		"chainId":           config.ChainId,
		"iConsensusAddress": config.IConsensusAddress,
	}

	_, err := pg.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInsertRow, err)
	}

	return nil
}

func (pg *Database) InsertApplication(
	ctx context.Context,
	app *Application,
) error {
	query := `
	INSERT INTO application
		(contract_address,
		template_hash,
		snapshot_uri,
		last_processed_block,
		epoch_length,
		status)
	VALUES
		(@contractAddress,
		@templateHash,
		@snapshotUri,
		@lastProcessedBlock,
		@epochLength,
		@status)`

	args := pgx.NamedArgs{
		"contractAddress":    app.ContractAddress,
		"templateHash":       app.TemplateHash,
		"snapshotUri":        app.SnapshotURI,
		"lastProcessedBlock": app.LastProcessedBlock,
		"epochLength":        app.EpochLength,
		"status":             app.Status,
	}

	_, err := pg.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInsertRow, err)
	}

	return nil
}

func (pg *Database) InsertInput(
	ctx context.Context,
	input *Input,
) error {
	query := `
	INSERT INTO input
		(index,
		status,
		raw_data,
		block_number,
		machine_hash,
		outputs_hash,
		application_address)
	VALUES
		(@index,
		@status,
		@rawData,
		@blockNumber,
		@machineHash,
		@outputsHash,
		@applicationAddress)`

	args := pgx.NamedArgs{
		"index":              input.Index,
		"status":             input.CompletionStatus,
		"rawData":            input.RawData,
		"blockNumber":        input.BlockNumber,
		"machineHash":        input.MachineHash,
		"outputsHash":        input.OutputsHash,
		"applicationAddress": input.AppAddress,
	}

	_, err := pg.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInsertRow, err)
	}

	return nil
}

func (pg *Database) InsertOutput(
	ctx context.Context,
	output *Output,
) error {
	query := `
	INSERT INTO output
		(index,
		raw_data,
		output_hashes_siblings,
		input_id)
	VALUES
		(@index,
		@rawData,
		@outputHashesSiblings,
		@inputId)`

	args := pgx.NamedArgs{
		"inputId":              output.InputId,
		"index":                output.Index,
		"rawData":              output.RawData,
		"outputHashesSiblings": output.OutputHashesSiblings,
	}

	_, err := pg.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInsertRow, err)
	}

	return nil
}

func (pg *Database) InsertReport(
	ctx context.Context,
	report *Report,
) error {
	query := `
	INSERT INTO report
		(index,
		raw_data,
		input_id)
	VALUES
		(@index,
		@rawData,
		@inputId)`

	args := pgx.NamedArgs{
		"inputId": report.InputId,
		"index":   report.Index,
		"rawData": report.RawData,
	}

	_, err := pg.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInsertRow, err)
	}

	return nil
}

func (pg *Database) InsertClaim(
	ctx context.Context,
	claim *Claim,
) error {
	query := `
	INSERT INTO claim
		(index,
		output_merkle_root_hash,
		transaction_hash,
		status,
		application_address)
	VALUES
		(@index,
		@outputMerkleRootHash,
		@transactionHash,
		@status,
		@applicationAddress)`

	args := pgx.NamedArgs{
		"index":                claim.Index,
		"outputMerkleRootHash": claim.OutputMerkleRootHash,
		"transactionHash":      claim.TransactionHash,
		"status":               claim.Status,
		"applicationAddress":   claim.AppAddress,
	}

	_, err := pg.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInsertRow, err)
	}

	return nil
}

func (pg *Database) GetNodeConfig(
	ctx context.Context,
) (*NodePersistentConfig, error) {
	var (
		defaultBlock      DefaultBlock
		deploymentBlock   uint64
		inputBoxAddress   Address
		chainId           uint64
		iConsensusAddress Address
	)

	query := `
	SELECT
		default_block,
		input_box_deployment_block,
		input_box_address,
		chain_id,
		iconsensus_address
	FROM
		node_config`

	err := pg.db.QueryRow(ctx, query).Scan(
		&defaultBlock,
		&deploymentBlock,
		&inputBoxAddress,
		&chainId,
		&iConsensusAddress,
	)
	if err != nil {
		return nil, fmt.Errorf("GetNodeConfig QueryRow failed: %w\n", err)
	}

	config := NodePersistentConfig{
		DefaultBlock:            defaultBlock,
		InputBoxDeploymentBlock: deploymentBlock,
		InputBoxAddress:         inputBoxAddress,
		ChainId:                 chainId,
		IConsensusAddress:       iConsensusAddress,
	}

	return &config, nil
}

func (pg *Database) GetApplication(
	ctx context.Context,
	appAddressKey Address,
) (*Application, error) {
	var (
		id                 uint64
		contractAddress    Address
		templateHash       Hash
		snapshotUri        string
		lastProcessedBlock uint64
		epochLength        uint64
		status             ApplicationStatus
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
		contract_address=@contractAddress`

	args := pgx.NamedArgs{
		"contractAddress": appAddressKey,
	}

	err := pg.db.QueryRow(ctx, query, args).Scan(
		&id,
		&contractAddress,
		&templateHash,
		&snapshotUri,
		&lastProcessedBlock,
		&epochLength,
		&status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("GetApplication returned no rows", "service", "repository")
			return nil, nil
		}
		return nil, fmt.Errorf("GetApplication QueryRow failed: %w\n", err)
	}

	app := Application{
		Id:                 id,
		ContractAddress:    contractAddress,
		TemplateHash:       templateHash,
		SnapshotURI:        snapshotUri,
		LastProcessedBlock: lastProcessedBlock,
		EpochLength:        epochLength,
		Status:             status,
	}

	return &app, nil
}

func (pg *Database) GetInput(
	ctx context.Context,
	indexKey uint64,
	appAddressKey Address,
) (*Input, error) {
	var (
		id          uint64
		index       uint64
		status      InputCompletionStatus
		rawData     []byte
		blockNumber uint64
		machineHash *Hash
		outputsHash *Hash
		appAddress  Address
	)

	query := `
	SELECT
		id,
		index,
		raw_data,
		status,
		block_number,
		machine_hash,
		outputs_hash,
		application_address
	FROM
		input
	WHERE
		index=@index and application_address=@appAddress`

	args := pgx.NamedArgs{
		"index":      indexKey,
		"appAddress": appAddressKey,
	}

	err := pg.db.QueryRow(ctx, query, args).Scan(
		&id,
		&index,
		&rawData,
		&status,
		&blockNumber,
		&machineHash,
		&outputsHash,
		&appAddress,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("GetInput returned no rows", "service", "repository")
			return nil, nil
		}
		return nil, fmt.Errorf("GetInput QueryRow failed: %w\n", err)
	}

	input := Input{
		Id:               id,
		Index:            index,
		CompletionStatus: status,
		RawData:          rawData,
		BlockNumber:      blockNumber,
		MachineHash:      machineHash,
		OutputsHash:      outputsHash,
		AppAddress:       appAddress,
	}

	return &input, nil
}

func (pg *Database) GetOutput(
	ctx context.Context,
	indexKey uint64,
	appAddressKey Address,
) (*Output, error) {
	var (
		id                   uint64
		index                uint64
		rawData              []byte
		hash                 *Hash
		outputHashesSiblings []Hash
		inputId              uint64
	)

	query := `
	SELECT
		o.id,
		o.index,
		o.raw_data,
		o.hash,
		o.output_hashes_siblings,
		o.input_id
	FROM
		output o
	INNER JOIN
		input i
	ON
		o.input_id=i.id
	WHERE
		o.index=@index and i.application_address=@appAddress`

	args := pgx.NamedArgs{
		"index":      indexKey,
		"appAddress": appAddressKey,
	}

	err := pg.db.QueryRow(ctx, query, args).Scan(
		&id,
		&index,
		&rawData,
		&hash,
		&outputHashesSiblings,
		&inputId,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("GetOutput returned no rows", "service", "repository")
			return nil, nil
		}
		return nil, fmt.Errorf("GetOutput QueryRow failed: %w\n", err)
	}

	output := Output{
		Id:                   id,
		Index:                index,
		RawData:              rawData,
		Hash:                 hash,
		OutputHashesSiblings: outputHashesSiblings,
		InputId:              inputId,
	}

	return &output, nil
}

func (pg *Database) GetReport(
	ctx context.Context,
	indexKey uint64,
	appAddressKey Address,
) (*Report, error) {
	var (
		id      uint64
		index   uint64
		rawData []byte
		inputId uint64
	)
	query := `
	SELECT
		r.id,
		r.index,
		r.raw_data,
		r.input_id
	FROM
		report r
	INNER JOIN
		input i
	ON
		r.input_id=i.id
	WHERE
		r.index=@index and i.application_address=@appAddress`

	args := pgx.NamedArgs{
		"index":      indexKey,
		"appAddress": appAddressKey,
	}
	err := pg.db.QueryRow(ctx, query, args).Scan(
		&id,
		&index,
		&rawData,
		&inputId,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("GetReport returned no rows", "service", "repository")
			return nil, nil
		}
		return nil, fmt.Errorf("GetReport QueryRow failed: %w\n", err)
	}

	report := Report{
		Id:      id,
		Index:   index,
		RawData: rawData,
		InputId: inputId,
	}

	return &report, nil
}

func (pg *Database) GetClaim(
	ctx context.Context,
	appAddressKey Address,
	indexKey uint64,
) (*Claim, error) {
	var (
		id                   uint64
		index                uint64
		outputMerkleRootHash Hash
		transactionHash      *Hash
		status               ClaimStatus
		address              Address
	)

	query := `
	SELECT
		id,
		index,
		output_merkle_root_hash,
		transaction_hash,
		status,
		application_address
	FROM
		claim
	WHERE
		application_address=@appAddress and index=@index`

	args := pgx.NamedArgs{
		"appAddress": appAddressKey,
		"index":      indexKey,
	}

	err := pg.db.QueryRow(ctx, query, args).Scan(
		&id,
		&index,
		&outputMerkleRootHash,
		&transactionHash,
		&status,
		&address,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("GetClaim returned no rows", "service", "repository")
			return nil, nil
		}
		return nil, fmt.Errorf("GetClaim QueryRow failed: %w\n", err)
	}

	claim := Claim{
		Id:                   id,
		Index:                index,
		OutputMerkleRootHash: outputMerkleRootHash,
		TransactionHash:      transactionHash,
		Status:               status,
		AppAddress:           address,
	}

	return &claim, nil
}
