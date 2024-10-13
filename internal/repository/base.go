// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/schema"
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

func ValidateSchema(endpoint string) error {

	schema, err := schema.New(endpoint)
	if err != nil {
		return err
	}
	defer schema.Close()

	_, err = schema.ValidateVersion()
	return err
}

// FIXME: improve this
func ValidateSchemaWithRetry(endpoint string, maxRetries int, delay time.Duration) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = ValidateSchema(endpoint)
		if err == nil {
			return nil
		}
		time.Sleep(delay)
	}
	return fmt.Errorf("failed to validate schema after %d attempts: %w", maxRetries, err)
}

func Connect(
	ctx context.Context,
	postgresEndpoint string,
) (*Database, error) {
	var (
		pgError    error
		pgInstance *Database
		pgOnce     sync.Once
	)

	pgError = ValidateSchemaWithRetry(postgresEndpoint, 5, 3*time.Second) // FIXME: get from config
	if pgError != nil {
		return nil, fmt.Errorf("unable to validate database schema version: %w\n", pgError)
	}

	pgOnce.Do(func() {
		dbpool, err := pgxpool.New(ctx, postgresEndpoint)
		if err != nil {
			pgError = fmt.Errorf("unable to create connection pool: %w\n", err)
			return
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
		last_claim_check_block,
		last_output_check_block,
		status,
		iconsensus_address)
	VALUES
		(@contractAddress,
		@templateHash,
		@templateUri,
		@lastProcessedBlock,
		@lastClaimCheckBlock,
		@lastOutputCheckBlock,
		@status,
		@iConsensusAddress)
	RETURNING
		id
	`
	args := pgx.NamedArgs{
		"contractAddress":      app.ContractAddress,
		"templateHash":         app.TemplateHash,
		"templateUri":          app.TemplateUri,
		"lastProcessedBlock":   app.LastProcessedBlock,
		"lastClaimCheckBlock":  app.LastClaimCheckBlock,
		"lastOutputCheckBlock": app.LastOutputCheckBlock,
		"status":               app.Status,
		"iConsensusAddress":    app.IConsensusAddress,
	}

	execParametersQuery := `
	INSERT INTO execution_parameters
		(application_id)
	VALUES
		(@applicationId)
	`

	tx, err := pg.db.Begin(ctx)
	if err != nil {
		return 0, errors.Join(ErrBeginTx, err)
	}

	var id uint64
	err = tx.QueryRow(ctx, query, args).Scan(&id)
	if err != nil {
		return 0, errors.Join(ErrInsertRow, err, tx.Rollback(ctx))
	}

	args = pgx.NamedArgs{"applicationId": id}
	_, err = tx.Exec(ctx, execParametersQuery, args)
	if err != nil {
		return 0, errors.Join(ErrInsertRow, err, tx.Rollback(ctx))
	}

	err = tx.Commit(ctx)
	if err != nil {
		return 0, errors.Join(ErrCommitTx, tx.Rollback(ctx))
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
		id`

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
		input_id,
		transaction_hash)
	VALUES
		(@index,
		@rawData,
		@hash,
		@outputHashesSiblings,
		@inputId,
		@transactionHash)
	RETURNING
		id
	`

	args := pgx.NamedArgs{
		"inputId":              output.InputId,
		"index":                output.Index,
		"rawData":              output.RawData,
		"hash":                 output.Hash,
		"outputHashesSiblings": output.OutputHashesSiblings,
		"transactionHash":      output.TransactionHash,
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
		id                   uint64
		contractAddress      Address
		templateHash         Hash
		templateUri          string
		lastProcessedBlock   uint64
		lastClaimCheckBlock  uint64
		lastOutputCheckBlock uint64
		status               ApplicationStatus
		iconsensusAddress    Address
	)

	query := `
	SELECT
		id,
		contract_address,
		template_hash,
		template_uri,
		last_processed_block,
		last_claim_check_block,
		last_output_check_block,
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
		&templateUri,
		&lastProcessedBlock,
		&lastClaimCheckBlock,
		&lastOutputCheckBlock,
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
		Id:                   id,
		ContractAddress:      contractAddress,
		TemplateHash:         templateHash,
		TemplateUri:          templateUri,
		LastProcessedBlock:   lastProcessedBlock,
		LastClaimCheckBlock:  lastClaimCheckBlock,
		LastOutputCheckBlock: lastOutputCheckBlock,
		Status:               status,
		IConsensusAddress:    iconsensusAddress,
	}

	return &app, nil
}

func (pg *Database) GetEpochs(ctx context.Context, application Address) ([]Epoch, error) {
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
		application_address=@appAddress
	ORDER BY
		index ASC`

	args := pgx.NamedArgs{
		"appAddress": application,
	}

	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("GetProcessedEpochs failed: %w", err)
	}

	var (
		id, index, firstBlock, lastBlock uint64
		appAddress                       Address
		claimHash, transactionHash       *Hash
		status                           string
		results                          []Epoch
	)

	scans := []any{
		&id, &appAddress, &index, &firstBlock, &lastBlock, &claimHash, &transactionHash, &status,
	}
	_, err = pgx.ForEachRow(rows, scans, func() error {
		epoch := Epoch{
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
		return nil, fmt.Errorf("GetProcessedEpochs failed: %w", err)
	}
	return results, nil
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

func (pg *Database) GetInputs(
	ctx context.Context,
	app Address,
) ([]*Input, error) {
	query := `
	SELECT   id, index, status, raw_data, epoch_id
	FROM     input
	WHERE    application_address = @applicationAddress
	ORDER BY index ASC
	`
	args := pgx.NamedArgs{
		"applicationAddress": app,
	}
	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("%w (failed querying inputs): %w", ErrAdvancerRepository, err)
	}

	res := []*Input{}
	var input Input
	scans := []any{&input.Id, &input.Index, &input.CompletionStatus, &input.RawData, &input.EpochId}
	_, err = pgx.ForEachRow(rows, scans, func() error {
		input := input
		res = append(res, &input)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w (failed reading input rows): %w", ErrAdvancerRepository, err)
	}

	return res, nil
}

func (pg *Database) GetInput(
	ctx context.Context,
	appAddressKey Address,
	indexKey uint64,
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

func (pg *Database) GetOutputs(
	ctx context.Context,
	application Address,
) ([]Output, error) {
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
		i.application_address=@appAddress
	ORDER BY
		o.index ASC
	`

	args := pgx.NamedArgs{"appAddress": application}
	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("GetOutputs failed: %w", err)
	}

	var (
		id, index, inputId   uint64
		rawData              []byte
		hash                 *Hash
		outputHashesSiblings []Hash
		outputs              []Output
	)
	scans := []any{&id, &index, &rawData, &hash, &outputHashesSiblings, &inputId}
	_, err = pgx.ForEachRow(rows, scans, func() error {
		output := Output{
			Id:                   id,
			Index:                index,
			RawData:              rawData,
			Hash:                 hash,
			OutputHashesSiblings: outputHashesSiblings,
			InputId:              inputId,
		}
		outputs = append(outputs, output)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("GetOutputs failed: %w", err)
	}
	return outputs, nil
}

func (pg *Database) GetOutputsByInputIndex(
	ctx context.Context,
	application Address,
	inputIndex uint64,
) ([]Output, error) {
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
		i.application_address=@appAddress
	AND
		i.index=@inputIndex
	ORDER BY
		o.index ASC
	`

	args := pgx.NamedArgs{
		"appAddress": application,
		"inputIndex": inputIndex,
	}
	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("GetOutputs failed: %w", err)
	}

	var (
		id, index, inputId   uint64
		rawData              []byte
		hash                 *Hash
		outputHashesSiblings []Hash
		outputs              []Output
	)
	scans := []any{&id, &index, &rawData, &hash, &outputHashesSiblings, &inputId}
	_, err = pgx.ForEachRow(rows, scans, func() error {
		output := Output{
			Id:                   id,
			Index:                index,
			RawData:              rawData,
			Hash:                 hash,
			OutputHashesSiblings: outputHashesSiblings,
			InputId:              inputId,
		}
		outputs = append(outputs, output)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("GetOutputs failed: %w", err)
	}
	return outputs, nil
}

func (pg *Database) GetOutput(
	ctx context.Context,
	appAddressKey Address,
	indexKey uint64,
) (*Output, error) {
	var (
		id                   uint64
		index                uint64
		rawData              []byte
		hash                 *Hash
		outputHashesSiblings []Hash
		inputId              uint64
		transactionHash      *Hash
	)

	query := `
	SELECT
		o.id,
		o.index,
		o.raw_data,
		o.hash,
		o.output_hashes_siblings,
		o.input_id,
		o.transaction_hash
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
		&transactionHash,
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
		TransactionHash:      transactionHash,
	}

	return &output, nil
}

func (pg *Database) GetReports(
	ctx context.Context,
	appAddressKey Address,
) ([]Report, error) {
	var (
		id      uint64
		index   uint64
		rawData []byte
		inputId uint64
		reports []Report
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
		i.application_address=@appAddress
	ORDER BY
		r.index ASC`

	args := pgx.NamedArgs{
		"appAddress": appAddressKey,
	}
	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("GetReports failed: %w", err)
	}

	scans := []any{&id, &index, &rawData, &inputId}
	_, err = pgx.ForEachRow(rows, scans, func() error {
		report := Report{
			Id:      id,
			Index:   index,
			RawData: rawData,
			InputId: inputId,
		}

		reports = append(reports, report)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("GetReports failed: %w", err)
	}

	return reports, nil
}

func (pg *Database) GetReportsByInputIndex(
	ctx context.Context,
	appAddressKey Address,
	inputIndex uint64,
) ([]Report, error) {
	var (
		id      uint64
		index   uint64
		rawData []byte
		inputId uint64
		reports []Report
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
		i.application_address=@appAddress
	AND
		i.index=@inputIndex
	ORDER BY
		r.index ASC`

	args := pgx.NamedArgs{
		"appAddress": appAddressKey,
		"inputIndex": inputIndex,
	}
	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("GetReports failed: %w", err)
	}

	scans := []any{&id, &index, &rawData, &inputId}
	_, err = pgx.ForEachRow(rows, scans, func() error {
		report := Report{
			Id:      id,
			Index:   index,
			RawData: rawData,
			InputId: inputId,
		}

		reports = append(reports, report)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("GetReports failed: %w", err)
	}

	return reports, nil
}

func (pg *Database) GetReport(
	ctx context.Context,
	appAddressKey Address,
	indexKey uint64,
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
