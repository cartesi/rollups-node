// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) CreateTournament(ctx context.Context, nameOrAddress string, value *Tournament) error {
	if value == nil {
		return fmt.Errorf("cannot create a nil tournament")
	}
	applicationID := table.Application.SELECT(table.Application.ID).WHERE(getWhereClauseFromNameOrAddress(nameOrAddress))
	stmt := tournamentUpsert(value, applicationID)
	sqlStr, args := stmt.Sql()
	command, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("%w: tournament %s", repository.ErrTournamentEventConflict, value.Address)
	}
	return nil
}

func (r *PostgresRepository) GetTournament(ctx context.Context, nameOrAddress string, address string) (*Tournament, error) {
	sel := table.Tournaments.SELECT(tournamentColumns, table.Tournaments.CreatedAt, table.Tournaments.UpdatedAt).
		FROM(table.Tournaments.INNER_JOIN(table.Application, table.Tournaments.ApplicationID.EQ(table.Application.ID))).
		WHERE(getWhereClauseFromNameOrAddress(nameOrAddress).AND(
			table.Tournaments.Address.EQ(postgres.Bytea(common.HexToAddress(address).Bytes())),
		))
	sqlStr, args := sel.Sql()
	value, err := scanTournament(r.db.QueryRow(ctx, sqlStr, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return value, err
}

func (r *PostgresRepository) ListTournaments(
	ctx context.Context, nameOrAddress string, f repository.TournamentFilter, p repository.Pagination, descending bool,
) ([]*Tournament, uint64, error) {
	if p.Limit > math.MaxInt64 || p.Offset > math.MaxInt64 {
		return nil, 0, fmt.Errorf("pagination exceeds PostgreSQL integer range")
	}
	from := table.Tournaments.INNER_JOIN(table.Application, table.Tournaments.ApplicationID.EQ(table.Application.ID))
	conditions := []postgres.BoolExpression{getWhereClauseFromNameOrAddress(nameOrAddress)}
	if f.EpochIndex != nil {
		conditions = append(conditions, table.Tournaments.EpochIndex.EQ(uint64Expr(*f.EpochIndex)))
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
	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	countStmt := table.Tournaments.SELECT(postgres.COUNT(postgres.STAR)).FROM(from).WHERE(postgres.AND(conditions...))
	total, err := countFromTx(ctx, tx, countStmt)
	if err != nil || total == 0 {
		return nil, total, err
	}
	sel := table.Tournaments.SELECT(tournamentColumns, table.Tournaments.CreatedAt, table.Tournaments.UpdatedAt).
		FROM(from).WHERE(postgres.AND(conditions...))
	if descending {
		sel = sel.ORDER_BY(table.Tournaments.EpochIndex.DESC(), table.Tournaments.Level.DESC(), table.Tournaments.Address.DESC())
	} else {
		sel = sel.ORDER_BY(table.Tournaments.EpochIndex.ASC(), table.Tournaments.Level.ASC(), table.Tournaments.Address.ASC())
	}
	if p.Limit > 0 {
		sel = sel.LIMIT(int64(p.Limit))
	}
	if p.Offset > 0 {
		sel = sel.OFFSET(int64(p.Offset))
	}
	sqlStr, args := sel.Sql()
	rows, err := tx.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var values []*Tournament
	for rows.Next() {
		value, err := scanTournament(rows)
		if err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return values, total, nil
}
