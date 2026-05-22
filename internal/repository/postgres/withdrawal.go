// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

type withdrawalExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// InsertWithdrawal records a Withdrawal event observed on chain. Idempotent on
// the (application_id, account_index) primary key via ON CONFLICT DO NOTHING,
// so re-processing the same block on restart cannot fail and cannot diverge
// from the first write. The contract marks each account index as withdrawn,
// so the event fires at most once per slot per app — duplicate inserts are
// restart artifacts.
func (r *PostgresRepository) InsertWithdrawal(
	ctx context.Context,
	w *model.Withdrawal,
) error {
	return insertWithdrawal(ctx, r.db, w)
}

func insertWithdrawal(ctx context.Context, exec withdrawalExecutor, w *model.Withdrawal) error {
	insertStmt := table.Withdrawal.
		INSERT(
			table.Withdrawal.ApplicationID,
			table.Withdrawal.AccountIndex,
			table.Withdrawal.Account,
			table.Withdrawal.Output,
			table.Withdrawal.BlockNumber,
			table.Withdrawal.TransactionHash,
			table.Withdrawal.LogIndex,
		).
		VALUES(
			w.ApplicationID,
			w.AccountIndex,
			w.Account,
			w.Output,
			w.BlockNumber,
			w.TransactionHash.Bytes(),
			int64(w.LogIndex),
		).
		ON_CONFLICT(table.Withdrawal.ApplicationID, table.Withdrawal.AccountIndex).
		DO_NOTHING()

	sqlStr, args := insertStmt.Sql()
	_, err := exec.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("insert withdrawal (app=%d, index=%d): %w", w.ApplicationID, w.AccountIndex, err)
	}
	return nil
}

// StoreWithdrawalEvents persists a completed withdrawal scan window atomically.
// Rows and cursor advancement are
// committed together so the DB withdrawal count remains the scanner's local
// previous counter for the next window.
func (r *PostgresRepository) StoreWithdrawalEvents(
	ctx context.Context,
	appID int64,
	withdrawals []*model.Withdrawal,
	blockNumber uint64,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, w := range withdrawals {
		if w.ApplicationID != appID {
			return fmt.Errorf("insert withdrawal (app=%d, index=%d): application id mismatch %d",
				w.ApplicationID, w.AccountIndex, appID)
		}
		if err := insertWithdrawal(ctx, tx, w); err != nil {
			return err
		}
	}

	updateStmt := table.Application.
		UPDATE(table.Application.LastWithdrawalCheckBlock).
		SET(uint64Expr(blockNumber)).
		WHERE(
			table.Application.ID.EQ(postgres.Int(appID)).
				AND(table.Application.LastWithdrawalCheckBlock.LT(uint64Expr(blockNumber))),
		)

	sqlStr, args := updateStmt.Sql()
	if _, err := tx.Exec(ctx, sqlStr, args...); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GetNumberOfWithdrawals returns COUNT(*) of withdrawal rows for the app. The
// post-foreclosure scanner uses this as the previous on-chain withdrawal
// counter, which is only correct because withdrawal rows are append-only (see
// the note on the "withdrawal" table in the schema migration). If rows could be
// deleted, this count would fall below the chain counter and the scanner's
// monotonic guard would (correctly) flag corruption.
func (r *PostgresRepository) GetNumberOfWithdrawals(
	ctx context.Context,
	appID int64,
) (uint64, error) {
	sel := table.Withdrawal.
		SELECT(postgres.COUNT(postgres.STAR)).
		FROM(table.Withdrawal).
		WHERE(table.Withdrawal.ApplicationID.EQ(postgres.Int(appID)))

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var count uint64
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *PostgresRepository) GetWithdrawal(
	ctx context.Context,
	nameOrAddress string,
	accountIndex uint64,
) (*model.Withdrawal, error) {
	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	sel := table.Withdrawal.
		SELECT(
			table.Withdrawal.ApplicationID,
			table.Withdrawal.AccountIndex,
			table.Withdrawal.Account,
			table.Withdrawal.Output,
			table.Withdrawal.BlockNumber,
			table.Withdrawal.TransactionHash,
			table.Withdrawal.LogIndex,
			table.Withdrawal.CreatedAt,
			table.Withdrawal.UpdatedAt,
		).
		FROM(
			table.Withdrawal.INNER_JOIN(
				table.Application,
				table.Withdrawal.ApplicationID.EQ(table.Application.ID),
			),
		).
		WHERE(
			whereClause.
				AND(table.Withdrawal.AccountIndex.EQ(uint64Expr(accountIndex))),
		)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var w model.Withdrawal
	var txHashBytes []byte
	var logIndex int64
	err := row.Scan(
		&w.ApplicationID,
		&w.AccountIndex,
		&w.Account,
		&w.Output,
		&w.BlockNumber,
		&txHashBytes,
		&logIndex,
		&w.CreatedAt,
		&w.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	copy(w.TransactionHash[:], txHashBytes)
	w.LogIndex = uint(logIndex)
	return &w, nil
}

func (r *PostgresRepository) ListWithdrawals(
	ctx context.Context,
	nameOrAddress string,
	f repository.WithdrawalFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Withdrawal, uint64, error) {
	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	fromClause := table.Withdrawal.INNER_JOIN(
		table.Application,
		table.Withdrawal.ApplicationID.EQ(table.Application.ID),
	)

	conditions := []postgres.BoolExpression{whereClause}
	if f.AccountIndex != nil {
		conditions = append(conditions, table.Withdrawal.AccountIndex.EQ(uint64Expr(*f.AccountIndex)))
	}

	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	countStmt := table.Withdrawal.SELECT(postgres.COUNT(postgres.STAR)).
		FROM(fromClause).WHERE(postgres.AND(conditions...))
	total, err := countFromTx(ctx, tx, countStmt)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	sel := table.Withdrawal.
		SELECT(
			table.Withdrawal.ApplicationID,
			table.Withdrawal.AccountIndex,
			table.Withdrawal.Account,
			table.Withdrawal.Output,
			table.Withdrawal.BlockNumber,
			table.Withdrawal.TransactionHash,
			table.Withdrawal.LogIndex,
			table.Withdrawal.CreatedAt,
			table.Withdrawal.UpdatedAt,
		).
		FROM(fromClause).
		WHERE(postgres.AND(conditions...))

	if descending {
		sel = sel.ORDER_BY(table.Withdrawal.AccountIndex.DESC())
	} else {
		sel = sel.ORDER_BY(table.Withdrawal.AccountIndex.ASC())
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

	var withdrawals []*model.Withdrawal
	for rows.Next() {
		var w model.Withdrawal
		var txHashBytes []byte
		var logIndex int64
		err := rows.Scan(
			&w.ApplicationID,
			&w.AccountIndex,
			&w.Account,
			&w.Output,
			&w.BlockNumber,
			&txHashBytes,
			&logIndex,
			&w.CreatedAt,
			&w.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		copy(w.TransactionHash[:], txHashBytes)
		w.LogIndex = uint(logIndex)
		withdrawals = append(withdrawals, &w)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return withdrawals, total, nil
}
