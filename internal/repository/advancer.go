// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	. "github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/internal/nodemachine"
	"github.com/jackc/pgx/v5"
)

var ErrAdvancerRepository = errors.New("advancer repository error")

type AdvancerRepository struct{ *Database }

func (repo *AdvancerRepository) GetUnprocessedInputs(
	ctx context.Context,
	apps []Address,
) (map[Address][]*Input, error) {
	result := map[Address][]*Input{}
	if len(apps) == 0 {
		return result, nil
	}

	query := fmt.Sprintf(`
        SELECT   id, application_address, raw_data
        FROM     input
        WHERE    status = 'NONE'
        AND      application_address IN %s 
        ORDER BY index ASC, application_address
    `, toSqlIn(apps)) // NOTE: not sanitized
	rows, err := repo.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w (failed querying inputs): %w", ErrAdvancerRepository, err)
	}

	var input Input
	scans := []any{&input.Id, &input.AppAddress, &input.RawData}
	_, err = pgx.ForEachRow(rows, scans, func() error {
		input := input
		if _, ok := result[input.AppAddress]; ok { //nolint:gosimple
			result[input.AppAddress] = append(result[input.AppAddress], &input)
		} else {
			result[input.AppAddress] = []*Input{&input}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w (failed reading input rows): %w", ErrAdvancerRepository, err)
	}

	return result, nil
}

func (repo *AdvancerRepository) StoreAdvanceResult(
	ctx context.Context,
	input *Input,
	res *nodemachine.AdvanceResult,
) error {
	tx, err := repo.db.Begin(ctx)
	if err != nil {
		return errors.Join(ErrBeginTx, err)
	}

	// Inserts the outputs.
	nextOutputIndex, err := repo.getNextIndex(ctx, tx, "output", input.AppAddress)
	if err != nil {
		return err
	}
	err = repo.insert(ctx, tx, "output", res.Outputs, input.Id, nextOutputIndex)
	if err != nil {
		return err
	}

	// Inserts the reports.
	nextReportIndex, err := repo.getNextIndex(ctx, tx, "report", input.AppAddress)
	if err != nil {
		return err
	}
	err = repo.insert(ctx, tx, "report", res.Reports, input.Id, nextReportIndex)
	if err != nil {
		return err
	}

	// Updates the input's status.
	err = repo.updateInput(ctx, tx, input.Id, res.Status, res.OutputsHash, res.MachineHash)
	if err != nil {
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return errors.Join(ErrCommitTx, err, tx.Rollback(ctx))
	}

	return nil
}

func (repo *AdvancerRepository) UpdateEpochs(ctx context.Context, app Address) error {
	query := `
        UPDATE epoch
        SET    status = 'PROCESSED_ALL_INPUTS'
        WHERE  id IN ((
            SELECT DISTINCT epoch.id
            FROM   epoch INNER JOIN input ON (epoch.id = input.epoch_id)
            WHERE  epoch.application_address = @applicationAddress
            AND    epoch.status = 'CLOSED'
            AND    input.status != 'NONE'
        ) EXCEPT (
            SELECT DISTINCT epoch.id
            FROM   epoch INNER JOIN input ON (epoch.id = input.epoch_id)
            WHERE  epoch.application_address = @applicationAddress
            AND    epoch.status = 'CLOSED'
            AND    input.status = 'NONE'))
    `
	args := pgx.NamedArgs{"applicationAddress": app}
	_, err := repo.db.Exec(ctx, query, args)
	if err != nil {
		return errors.Join(ErrUpdateRow, err)
	}
	return nil
}

// ------------------------------------------------------------------------------------------------

func (_ *AdvancerRepository) getNextIndex(
	ctx context.Context,
	tx pgx.Tx,
	tableName string,
	appAddress Address,
) (uint64, error) {
	var nextIndex uint64
	query := fmt.Sprintf(`
        SELECT COALESCE(MAX(%s.index) + 1, 0)
        FROM   input INNER JOIN %s ON input.id = %s.input_id
        WHERE  input.status = 'ACCEPTED'
        AND    input.application_address = $1
	`, tableName, tableName, tableName)
	err := tx.QueryRow(ctx, query, appAddress).Scan(&nextIndex)
	if err != nil {
		err = fmt.Errorf("failed to get the next %s index: %w", tableName, err)
		return 0, errors.Join(err, tx.Rollback(ctx))
	}
	return nextIndex, nil
}

func (_ *AdvancerRepository) insert(
	ctx context.Context,
	tx pgx.Tx,
	tableName string,
	dataArray [][]byte,
	inputId uint64,
	nextIndex uint64,
) error {
	lenOutputs := int64(len(dataArray))
	if lenOutputs < 1 {
		return nil
	}

	rows := [][]any{}
	for i, data := range dataArray {
		rows = append(rows, []any{inputId, nextIndex + uint64(i), data})
	}

	count, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{tableName},
		[]string{"input_id", "index", "raw_data"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return errors.Join(ErrCopyFrom, err, tx.Rollback(ctx))
	}
	if lenOutputs != count {
		err := fmt.Errorf("not all %ss were inserted (%d != %d)", tableName, lenOutputs, count)
		return errors.Join(err, tx.Rollback(ctx))
	}

	return nil
}

func (_ *AdvancerRepository) updateInput(
	ctx context.Context,
	tx pgx.Tx,
	inputId uint64,
	status InputCompletionStatus,
	outputsHash Hash,
	machineHash *Hash,
) error {
	query := `
        UPDATE input
        SET    (status, outputs_hash, machine_hash) = (@status, @outputsHash, @machineHash)
        WHERE  id = @id
    `
	args := pgx.NamedArgs{
		"status":      status,
		"outputsHash": outputsHash,
		"machineHash": machineHash,
		"id":          inputId,
	}
	_, err := tx.Exec(ctx, query, args)
	if err != nil {
		return errors.Join(ErrUpdateRow, err, tx.Rollback(ctx))
	}
	return nil
}

// ------------------------------------------------------------------------------------------------

func toSqlIn[T fmt.Stringer](a []T) string {
	s := []string{}
	for _, x := range a {
		s = append(s, fmt.Sprintf("'\\x%s'", x.String()[2:]))
	}
	return fmt.Sprintf("(%s)", strings.Join(s, ", "))
}
