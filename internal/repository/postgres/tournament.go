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
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/enum"
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

func (r *PostgresRepository) ListRecoverableRootBonds(
	ctx context.Context, appID int64, claimer common.Address, observedBlock uint64,
	afterEpoch *uint64, throughEpoch uint64, limit uint64,
) ([]repository.RootBondRecoveryCandidate, error) {
	if limit == 0 || limit > math.MaxInt64 {
		return nil, fmt.Errorf("root bond recovery limit must be between 1 and %d", int64(math.MaxInt64))
	}
	if afterEpoch != nil && *afterEpoch >= throughEpoch {
		return nil, nil
	}
	conditions := []postgres.BoolExpression{
		table.Tournaments.ApplicationID.EQ(postgres.Int64(appID)),
		// Keep the root predicate literal so generic plans can use the partial root index.
		table.Tournaments.Level.EQ(postgres.RawInt("0")),
		table.Tournaments.BondDisposition.EQ(enum.BondDisposition.Recoverable),
		table.Tournaments.BondClaimer.EQ(postgres.Bytea(claimer.Bytes())),
		table.Tournaments.AsOfBlock.LT_EQ(uint64Expr(observedBlock)),
		table.Tournaments.EpochIndex.LT_EQ(uint64Expr(throughEpoch)),
	}
	if afterEpoch != nil {
		conditions = append(conditions, table.Tournaments.EpochIndex.GT(uint64Expr(*afterEpoch)))
	}
	stmt := table.Tournaments.SELECT(table.Tournaments.EpochIndex, table.Tournaments.Address).
		WHERE(postgres.AND(conditions...)).ORDER_BY(table.Tournaments.EpochIndex.ASC()).LIMIT(int64(limit))
	sqlStr, args := stmt.Sql()
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("listing recoverable root bonds for application %d: %w", appID, err)
	}
	defer rows.Close()
	var candidates []repository.RootBondRecoveryCandidate
	for rows.Next() {
		var candidate repository.RootBondRecoveryCandidate
		if err := rows.Scan(&candidate.EpochIndex, &candidate.Tournament); err != nil {
			return nil, fmt.Errorf("reading recoverable root bond for application %d: %w", appID, err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing recoverable root bonds for application %d: %w", appID, err)
	}
	return candidates, nil
}
