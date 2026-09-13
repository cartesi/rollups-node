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

func (r *PostgresRepository) CreateCommitment(ctx context.Context, nameOrAddress string, value *Commitment) error {
	if value == nil {
		return fmt.Errorf("cannot create a nil commitment")
	}
	applicationID := table.Application.SELECT(table.Application.ID).WHERE(getWhereClauseFromNameOrAddress(nameOrAddress))
	values := commitmentValues(value)
	values[0] = applicationID
	stmt := table.Commitments.INSERT(commitmentColumns).VALUES(values[0], values[1:]...)
	sqlStr, args := stmt.Sql()
	_, err := r.db.Exec(ctx, sqlStr, args...)
	return err
}

func (r *PostgresRepository) GetCommitment(
	ctx context.Context, nameOrAddress string, epochIndex uint64, tournamentAddress string, commitmentHex string,
) (*Commitment, error) {
	sel := table.Commitments.SELECT(commitmentColumns, table.Commitments.CreatedAt, table.Commitments.UpdatedAt).
		FROM(table.Commitments.INNER_JOIN(table.Application, table.Commitments.ApplicationID.EQ(table.Application.ID))).
		WHERE(getWhereClauseFromNameOrAddress(nameOrAddress).AND(
			table.Commitments.EpochIndex.EQ(uint64Expr(epochIndex)).
				AND(table.Commitments.TournamentAddress.EQ(postgres.Bytea(common.HexToAddress(tournamentAddress).Bytes()))).
				AND(table.Commitments.Commitment.EQ(postgres.Bytea(common.HexToHash(commitmentHex).Bytes()))),
		))
	sqlStr, args := sel.Sql()
	value, err := scanCommitment(r.db.QueryRow(ctx, sqlStr, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return value, err
}

func (r *PostgresRepository) ListCommitments(
	ctx context.Context, nameOrAddress string, f repository.CommitmentFilter, p repository.Pagination, descending bool,
) ([]*Commitment, uint64, error) {
	if p.Limit > math.MaxInt64 || p.Offset > math.MaxInt64 {
		return nil, 0, fmt.Errorf("pagination exceeds PostgreSQL integer range")
	}
	from := table.Commitments.INNER_JOIN(table.Application, table.Commitments.ApplicationID.EQ(table.Application.ID))
	conditions := []postgres.BoolExpression{getWhereClauseFromNameOrAddress(nameOrAddress)}
	if f.EpochIndex != nil {
		conditions = append(conditions, table.Commitments.EpochIndex.EQ(uint64Expr(*f.EpochIndex)))
	}
	if f.TournamentAddress != nil {
		conditions = append(conditions,
			table.Commitments.TournamentAddress.EQ(postgres.Bytea(common.HexToAddress(*f.TournamentAddress).Bytes())))
	}
	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	countStmt := table.Commitments.SELECT(postgres.COUNT(postgres.STAR)).FROM(from).WHERE(postgres.AND(conditions...))
	total, err := countFromTx(ctx, tx, countStmt)
	if err != nil || total == 0 {
		return nil, total, err
	}
	sel := table.Commitments.SELECT(commitmentColumns, table.Commitments.CreatedAt, table.Commitments.UpdatedAt).
		FROM(from).WHERE(postgres.AND(conditions...))
	if descending {
		sel = sel.ORDER_BY(
			table.Commitments.EpochIndex.DESC(), table.Commitments.TournamentAddress.DESC(), table.Commitments.Commitment.DESC())
	} else {
		sel = sel.ORDER_BY(
			table.Commitments.EpochIndex.ASC(), table.Commitments.TournamentAddress.ASC(), table.Commitments.Commitment.ASC())
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
	var values []*Commitment
	for rows.Next() {
		value, err := scanCommitment(rows)
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
