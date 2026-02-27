// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

// ------------------------ TournamentRepository Methods ------------------------ //

func (r *PostgresRepository) CreateTournament(
	ctx context.Context,
	nameOrAddress string,
	t *model.Tournament,
) error {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return err
	}

	insertStmt := table.Tournaments.
		INSERT(
			table.Tournaments.ApplicationID,
			table.Tournaments.EpochIndex,
			table.Tournaments.Address,
			table.Tournaments.ParentTournamentAddress,
			table.Tournaments.ParentMatchIDHash,
			table.Tournaments.MaxLevel,
			table.Tournaments.Level,
			table.Tournaments.Log2step,
			table.Tournaments.Height,
			table.Tournaments.WinnerCommitment,
			table.Tournaments.FinalStateHash,
			table.Tournaments.FinishedAtBlock,
		)

	parentAddress := postgres.NULL
	if t.ParentTournamentAddress != nil {
		parentAddress = postgres.Bytea(t.ParentTournamentAddress.Bytes())
	}
	parentMatch := postgres.NULL
	if t.ParentMatchIDHash != nil {
		parentMatch = postgres.Bytea(t.ParentMatchIDHash.Bytes())
	}
	winnerCommitment := postgres.NULL
	if t.WinnerCommitment != nil {
		winnerCommitment = postgres.Bytea(t.WinnerCommitment.Bytes())
	}
	finalState := postgres.NULL
	if t.FinalStateHash != nil {
		finalState = postgres.Bytea(t.FinalStateHash.Bytes())
	}

	selectQuery := table.Application.SELECT(
		table.Application.ID,
		postgres.RawFloat(fmt.Sprintf("%d", t.EpochIndex)),
		postgres.Bytea(t.Address.Bytes()),
		parentAddress,
		parentMatch,
		postgres.RawFloat(fmt.Sprintf("%d", t.MaxLevel)),
		postgres.RawFloat(fmt.Sprintf("%d", t.Level)),
		postgres.RawFloat(fmt.Sprintf("%d", t.Log2Step)),
		postgres.RawFloat(fmt.Sprintf("%d", t.Height)),
		winnerCommitment,
		finalState,
		postgres.RawFloat(fmt.Sprintf("%d", t.FinishedAtBlock)),
	).WHERE(
		whereClause,
	)

	sqlStr, args := insertStmt.QUERY(selectQuery).Sql()
	_, err = r.db.Exec(ctx, sqlStr, args...)

	return err
}

func (r *PostgresRepository) UpdateTournament(
	ctx context.Context,
	nameOrAddress string,
	t *model.Tournament,
) error {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return err
	}

	winnerCommitment := postgres.NULL
	if t.WinnerCommitment != nil {
		winnerCommitment = postgres.Bytea(t.WinnerCommitment.Bytes())
	}
	finalState := postgres.NULL
	if t.FinalStateHash != nil {
		finalState = postgres.Bytea(t.FinalStateHash.Bytes())
	}

	updateStmt := table.Tournaments.
		UPDATE(
			table.Tournaments.WinnerCommitment,
			table.Tournaments.FinalStateHash,
			table.Tournaments.FinishedAtBlock,
		).
		SET(
			winnerCommitment,
			finalState,
			t.FinishedAtBlock,
		).
		FROM(
			table.Application,
		).
		WHERE(postgres.AND(
			whereClause,
			table.Tournaments.ApplicationID.EQ(postgres.Int(t.ApplicationID)),
			table.Tournaments.EpochIndex.EQ(postgres.RawFloat(fmt.Sprintf("%d", t.EpochIndex))),
			table.Tournaments.Address.EQ(postgres.Bytea(t.Address.Bytes())),
		))

	sqlStr, args := updateStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PostgresRepository) GetTournament(
	ctx context.Context,
	nameOrAddress string,
	address string,
) (*model.Tournament, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, err
	}

	tournamentAddress := common.HexToAddress(address)
	sel := table.Tournaments.
		SELECT(
			table.Tournaments.ApplicationID,
			table.Tournaments.EpochIndex,
			table.Tournaments.Address,
			table.Tournaments.ParentTournamentAddress,
			table.Tournaments.ParentMatchIDHash,
			table.Tournaments.MaxLevel,
			table.Tournaments.Level,
			table.Tournaments.Log2step,
			table.Tournaments.Height,
			table.Tournaments.WinnerCommitment,
			table.Tournaments.FinalStateHash,
			table.Tournaments.FinishedAtBlock,
			table.Tournaments.CreatedAt,
			table.Tournaments.UpdatedAt,
		).
		FROM(
			table.Tournaments.
				INNER_JOIN(table.Application,
					table.Tournaments.ApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.Tournaments.Address.EQ(postgres.Bytea(tournamentAddress.Bytes()))),
		)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var t model.Tournament
	err = row.Scan(
		&t.ApplicationID,
		&t.EpochIndex,
		&t.Address,
		&t.ParentTournamentAddress,
		&t.ParentMatchIDHash,
		&t.MaxLevel,
		&t.Level,
		&t.Log2Step,
		&t.Height,
		&t.WinnerCommitment,
		&t.FinalStateHash,
		&t.FinishedAtBlock,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *PostgresRepository) ListTournaments(
	ctx context.Context,
	nameOrAddress string,
	f repository.TournamentFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Tournament, uint64, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, 0, err
	}

	sel := table.Tournaments.
		SELECT(
			table.Tournaments.ApplicationID,
			table.Tournaments.EpochIndex,
			table.Tournaments.Address,
			table.Tournaments.ParentTournamentAddress,
			table.Tournaments.ParentMatchIDHash,
			table.Tournaments.MaxLevel,
			table.Tournaments.Level,
			table.Tournaments.Log2step,
			table.Tournaments.Height,
			table.Tournaments.WinnerCommitment,
			table.Tournaments.FinalStateHash,
			table.Tournaments.FinishedAtBlock,
			table.Tournaments.CreatedAt,
			table.Tournaments.UpdatedAt,
			postgres.COUNT(postgres.STAR).OVER().AS("total_count"),
		).
		FROM(
			table.Tournaments.
				INNER_JOIN(table.Application,
					table.Tournaments.ApplicationID.EQ(table.Application.ID),
				),
		)

	conditions := []postgres.BoolExpression{whereClause}
	if f.EpochIndex != nil {
		conditions = append(conditions, table.Tournaments.EpochIndex.EQ(postgres.RawFloat(fmt.Sprintf("%d", *f.EpochIndex))))
	}
	if f.Level != nil {
		conditions = append(conditions, table.Tournaments.Level.EQ(postgres.RawInt(fmt.Sprintf("%d", *f.Level))))
	}
	if f.ParentTournamentAddress != nil {
		conditions = append(conditions, table.Tournaments.ParentTournamentAddress.EQ(postgres.Bytea(f.ParentTournamentAddress.Bytes())))
	}
	if f.ParentMatchIDHash != nil {
		conditions = append(conditions, table.Tournaments.ParentMatchIDHash.EQ(postgres.Bytea(f.ParentMatchIDHash.Bytes())))
	}

	sel = sel.WHERE(postgres.AND(conditions...))

	if descending {
		sel = sel.ORDER_BY(table.Tournaments.EpochIndex.DESC(), table.Tournaments.Level.DESC())
	} else {
		sel = sel.ORDER_BY(table.Tournaments.EpochIndex.ASC(), table.Tournaments.Level.ASC())
	}

	// Apply pagination
	if p.Limit > 0 {
		sel = sel.LIMIT(int64(p.Limit))
	}
	if p.Offset > 0 {
		sel = sel.OFFSET(int64(p.Offset))
	}

	sqlStr, args := sel.Sql()
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tournaments []*model.Tournament
	var total uint64
	for rows.Next() {
		var t model.Tournament
		err := rows.Scan(
			&t.ApplicationID,
			&t.EpochIndex,
			&t.Address,
			&t.ParentTournamentAddress,
			&t.ParentMatchIDHash,
			&t.MaxLevel,
			&t.Level,
			&t.Log2Step,
			&t.Height,
			&t.WinnerCommitment,
			&t.FinalStateHash,
			&t.FinishedAtBlock,
			&t.CreatedAt,
			&t.UpdatedAt,
			&total,
		)
		if err != nil {
			return nil, 0, err
		}
		tournaments = append(tournaments, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return tournaments, total, nil
}
