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

var (
	ErrInsertRow = errors.New("unable to insert row")
	ErrUpdateRow = errors.New("unable to update row")
	ErrCopyFrom  = errors.New("unable to COPY FROM")

	ErrBeginTx  = errors.New("unable to begin transaction")
	ErrCommitTx = errors.New("unable to commit transaction")
)

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
		chain_id)
	SELECT
		@defaultBlock,
		@deploymentBlock,
		@inputBoxAddress,
		@chainId
	WHERE NOT EXISTS (SELECT * FROM node_config)`

	args := pgx.NamedArgs{
		"defaultBlock":    config.DefaultBlock,
		"deploymentBlock": config.InputBoxDeploymentBlock,
		"inputBoxAddress": config.InputBoxAddress,
		"chainId":         config.ChainId,
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
) (uint64, error) {
	query := `
	INSERT INTO application
		(contract_address,
		template_hash,
		template_uri,
		last_processed_block,
		status,
		iconsensus_address,
        machine_inc_cycles,
        machine_max_cycles,
        machine_advance_timeout,
        machine_inspect_timeout,
        machine_max_concurrent_inspects)
	VALUES
		(@contractAddress,
		@templateHash,
		@templateUri,
		@lastProcessedBlock,
		@status,
		@iConsensusAddress,
        @machineIncCycles,
        @machineMaxCycles,
        @machineAdvanceTimeout,
        @machineInspectTimeout,
        @machineMaxConcurrentInspects)
    RETURNING
        id
    `

	args := pgx.NamedArgs{
		"contractAddress":              app.ContractAddress,
		"templateHash":                 app.TemplateHash,
		"templateUri":                  app.TemplateUri,
		"lastProcessedBlock":           app.LastProcessedBlock,
		"status":                       app.Status,
		"iConsensusAddress":            app.IConsensusAddress,
		"machineIncCycles":             app.MachineIncCycles,
		"machineMaxCycles":             app.MachineMaxCycles,
		"machineAdvanceTimeout":        app.MachineAdvanceTimeout,
		"machineInspectTimeout":        app.MachineInspectTimeout,
		"machineMaxConcurrentInspects": app.MachineMaxConcurrentInspects,
	}

	var id uint64
	err := pg.db.QueryRow(ctx, query, args).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInsertRow, err)
	}

	return id, nil
}

func (pg *Database) InsertEpoch(
	ctx context.Context,
	epoch *Epoch,
) (uint64, error) {

	var id uint64

	query := `
	INSERT INTO epoch
		(index,
		first_block,
		last_block,
		transaction_hash,
		claim_hash,
		status,
		application_address)
	VALUES
		(@index,
		@firstBlock,
		@lastBlock,
		@transactionHash,
		@claimHash,
		@status,
		@applicationAddress)
	RETURNING
		id
    `

	args := pgx.NamedArgs{
		"index":              epoch.Index,
		"firstBlock":         epoch.FirstBlock,
		"lastBlock":          epoch.LastBlock,
		"transactionHash":    epoch.TransactionHash,
		"claimHash":          epoch.ClaimHash,
		"status":             epoch.Status,
		"applicationAddress": epoch.AppAddress,
	}

	err := pg.db.QueryRow(ctx, query, args).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInsertRow, err)
	}

	return id, nil
}

func (pg *Database) InsertInput(
	ctx context.Context,
	input *Input,
) (uint64, error) {
	query := `
	INSERT INTO input
		(index,
		status,
		raw_data,
		block_number,
		machine_hash,
		outputs_hash,
		application_address,
		epoch_id)
	VALUES
		(@index,
		@status,
		@rawData,
		@blockNumber,
		@machineHash,
		@outputsHash,
		@applicationAddress,
		@epochId)
	RETURNING
		id
	`

	args := pgx.NamedArgs{
		"index":              input.Index,
		"status":             input.CompletionStatus,
		"rawData":            input.RawData,
		"blockNumber":        input.BlockNumber,
		"machineHash":        input.MachineHash,
		"outputsHash":        input.OutputsHash,
		"applicationAddress": input.AppAddress,
		"epochId":            input.EpochId,
	}

	var id uint64
	err := pg.db.QueryRow(ctx, query, args).Scan(&id)
	if err != nil {
		return 0, errors.Join(ErrInsertRow, err)
	}

	return id, nil
}

func (pg *Database) InsertOutput(
	ctx context.Context,
	output *Output,
) (uint64, error) {
	query := `
	INSERT INTO output
		(index,
		raw_data,
		hash,
		output_hashes_siblings,
		input_id)
	VALUES
		(@index,
		@rawData,
		@hash,
		@outputHashesSiblings,
		@inputId)
	RETURNING
		id
	`

	args := pgx.NamedArgs{
		"inputId":              output.InputId,
		"index":                output.Index,
		"rawData":              output.RawData,
		"hash":                 output.Hash,
		"outputHashesSiblings": output.OutputHashesSiblings,
	}

	var id uint64
	err := pg.db.QueryRow(ctx, query, args).Scan(&id)
	if err != nil {
		return 0, errors.Join(ErrInsertRow, err)
	}

	return id, nil
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

func (pg *Database) InsertSnapshot(
	ctx context.Context,
	snapshot *Snapshot,
) (id uint64, _ error) {
	query := `
	INSERT INTO snapshot
		(input_id,
		application_address,
		uri)
	VALUES
		(@inputId,
		@appAddress,
		@uri)
	RETURNING
        id
    `

	args := pgx.NamedArgs{
		"inputId":    snapshot.InputId,
		"appAddress": snapshot.AppAddress,
		"uri":        snapshot.URI,
	}

	err := pg.db.QueryRow(ctx, query, args).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInsertRow, err)
	}

	return id, nil
}

func (pg *Database) GetNodeConfig(
	ctx context.Context,
) (*NodePersistentConfig, error) {
	var (
		defaultBlock    DefaultBlock
		deploymentBlock uint64
		inputBoxAddress Address
		chainId         uint64
	)

	query := `
	SELECT
		default_block,
		input_box_deployment_block,
		input_box_address,
		chain_id
	FROM
		node_config`

	err := pg.db.QueryRow(ctx, query).Scan(
		&defaultBlock,
		&deploymentBlock,
		&inputBoxAddress,
		&chainId,
	)
	if err != nil {
		return nil, fmt.Errorf("GetNodeConfig QueryRow failed: %w\n", err)
	}

	config := NodePersistentConfig{
		DefaultBlock:            defaultBlock,
		InputBoxDeploymentBlock: deploymentBlock,
		InputBoxAddress:         inputBoxAddress,
		ChainId:                 chainId,
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
		lastProcessedBlock uint64
		status             ApplicationStatus
		iconsensusAddress  Address
	)

	query := `
	SELECT
		id,
		contract_address,
		template_hash,
		last_processed_block,
		status,
		iconsensus_address
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
		&lastProcessedBlock,
		&status,
		&iconsensusAddress,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("GetApplication returned no rows",
				"service", "repository",
				"app", appAddressKey)
			return nil, nil
		}
		return nil, fmt.Errorf("GetApplication QueryRow failed: %w\n", err)
	}

	app := Application{
		Id:                 id,
		ContractAddress:    contractAddress,
		TemplateHash:       templateHash,
		LastProcessedBlock: lastProcessedBlock,
		Status:             status,
		IConsensusAddress:  iconsensusAddress,
	}

	return &app, nil
}

func (pg *Database) GetEpoch(
	ctx context.Context,
	indexKey uint64,
	appAddressKey Address,
) (*Epoch, error) {
	var (
		id                 uint64
		index              uint64
		firstBlock         uint64
		lastBlock          uint64
		transactionHash    *Hash
		claimHash          *Hash
		status             EpochStatus
		applicationAddress Address
	)

	query := `
	SELECT
		id,
		index,
		first_block,
		last_block,
		transaction_hash,
		claim_hash,
		status,
		application_address
	FROM
		epoch
	WHERE
		index=@index AND application_address=@appAddress`

	args := pgx.NamedArgs{
		"index":      indexKey,
		"appAddress": appAddressKey,
	}

	err := pg.db.QueryRow(ctx, query, args).Scan(
		&id,
		&index,
		&firstBlock,
		&lastBlock,
		&transactionHash,
		&claimHash,
		&status,
		&applicationAddress,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("GetEpoch returned no rows",
				"service", "repository",
				"app", appAddressKey,
				"epoch", indexKey)
			return nil, nil
		}
		return nil, fmt.Errorf("GetEpoch QueryRow failed: %w\n", err)
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
		epochId     uint64
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
		application_address,
		epoch_id
	FROM
		input
	WHERE
		index=@index AND application_address=@appAddress`

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
		&epochId,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("GetInput returned no rows",
				"service", "repository",
				"app", appAddressKey,
				"index", indexKey)
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
		EpochId:          epochId,
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
		o.index=@index AND i.application_address=@appAddress`

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
			slog.Debug("GetOutput returned no rows",
				"service", "repository",
				"app", appAddressKey,
				"index", indexKey)
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
		r.index=@index AND i.application_address=@appAddress`

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
			slog.Debug("GetReport returned no rows",
				"service", "repository",
				"app", appAddressKey,
				"index", indexKey)
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

func (pg *Database) GetSnapshot(
	ctx context.Context,
	inputIndexKey uint64,
	appAddressKey Address,
) (*Snapshot, error) {
	var (
		id         uint64
		inputId    uint64
		appAddress Address
		uri        string
	)
	query := `
	SELECT
		s.id,
		s.input_id,
		s.application_address,
		s.uri
	FROM
		snapshot s
	INNER JOIN
		input i
	ON
		i.id = s.input_id
	WHERE
		s.application_address=@appAddress AND i.index=@inputIndex
	`

	args := pgx.NamedArgs{
		"inputIndex": inputIndexKey,
		"appAddress": appAddressKey,
	}
	err := pg.db.QueryRow(ctx, query, args).Scan(
		&id,
		&inputId,
		&appAddress,
		&uri,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("GetSnapshot returned no rows",
				"service", "repository",
				"app", appAddressKey,
				"input index", inputIndexKey)
			return nil, nil
		}
		return nil, fmt.Errorf("GetSnapshot QueryRow failed: %w\n", err)
	}

	snapshot := Snapshot{
		Id:         id,
		InputId:    inputId,
		AppAddress: appAddress,
		URI:        uri,
	}

	return &snapshot, nil

}
