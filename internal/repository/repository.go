// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"fmt"
	"sync"

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
	postgres_endpoint string,
) (*database, error) {
	pgOnce.Do(func() {
		dbpool, err := pgxpool.New(ctx, postgres_endpoint)
		if err != nil {
			pgError = fmt.Errorf("unable to create connection pool: %w\n", err)
		}

		pgInstance = &database{dbpool}
	})

	return pgInstance, pgError
}

func (pg *database) Close() {
	pg.db.Close()
}

func (pg *database) InsertInput(
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

func (pg *database) GetInput(
	ctx context.Context,
	index int,
) (*Input, error) {
	var status string
	var blob []byte

	query := `
	SELECT 
		blob, 
		status
	FROM
		inputs
	WHERE
		index=$1`

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

func (pg *database) GetOutput(
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
