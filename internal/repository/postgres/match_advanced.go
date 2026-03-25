// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

// ------------------------ MatchAdvancedRepository Methods ------------------------ //

func (r *PostgresRepository) CreateMatchAdvanced(
	ctx context.Context,
	nameOrAddress string,
	m *model.MatchAdvanced,
) error {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	selectQuery := table.Application.SELECT(
		table.Application.ID,
		uint64Expr(m.EpochIndex),
		postgres.Bytea(m.TournamentAddress.Bytes()),
		postgres.Bytea(m.IDHash.Bytes()),
		postgres.Bytea(m.OtherParent.Bytes()),
		postgres.Bytea(m.LeftNode.Bytes()),
		uint64Expr(m.BlockNumber),
		postgres.Bytea(m.TxHash.Bytes()),
	).WHERE(
		whereClause,
	)

	insertStmt := table.MatchAdvances.INSERT(
		table.MatchAdvances.ApplicationID,
		table.MatchAdvances.EpochIndex,
		table.MatchAdvances.TournamentAddress,
		table.MatchAdvances.IDHash,
		table.MatchAdvances.OtherParent,
		table.MatchAdvances.LeftNode,
		table.MatchAdvances.BlockNumber,
		table.MatchAdvances.TxHash,
	).QUERY(
		selectQuery,
	)

	sqlStr, args := insertStmt.Sql()
	_, err := r.db.Exec(ctx, sqlStr, args...)

	return err
}

func (r *PostgresRepository) GetMatchAdvanced(
	ctx context.Context,
	nameOrAddress string,
	epochIndex uint64,
	tournamentAddress string,
	idHashHex string,
	parentHex string,
) (*model.MatchAdvanced, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	tournamentAddr := common.HexToAddress(tournamentAddress)
	idHash := common.HexToHash(idHashHex)
	parent := common.HexToHash(parentHex)

	sel := table.MatchAdvances.
		SELECT(
			table.MatchAdvances.ApplicationID,
			table.MatchAdvances.EpochIndex,
			table.MatchAdvances.TournamentAddress,
			table.MatchAdvances.IDHash,
			table.MatchAdvances.OtherParent,
			table.MatchAdvances.LeftNode,
			table.MatchAdvances.BlockNumber,
			table.MatchAdvances.TxHash,
			table.MatchAdvances.CreatedAt,
			table.MatchAdvances.UpdatedAt,
		).
		FROM(
			table.MatchAdvances.
				INNER_JOIN(table.Application,
					table.MatchAdvances.ApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.MatchAdvances.EpochIndex.EQ(uint64Expr(epochIndex))).
				AND(table.MatchAdvances.TournamentAddress.EQ(postgres.Bytea(tournamentAddr.Bytes()))).
				AND(table.MatchAdvances.IDHash.EQ(postgres.Bytea(idHash.Bytes()))).
				AND(table.MatchAdvances.OtherParent.EQ(postgres.Bytea(parent.Bytes()))),
		)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var m model.MatchAdvanced
	err := row.Scan(
		&m.ApplicationID,
		&m.EpochIndex,
		&m.TournamentAddress,
		&m.IDHash,
		&m.OtherParent,
		&m.LeftNode,
		&m.BlockNumber,
		&m.TxHash,
		&m.CreatedAt,
		&m.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *PostgresRepository) ListMatchAdvances(
	ctx context.Context,
	nameOrAddress string,
	epochIndex uint64,
	tournamentAddress string,
	idHashHex string,
	p repository.Pagination,
	descending bool,
) ([]*model.MatchAdvanced, uint64, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	fromClause := table.MatchAdvances.
		INNER_JOIN(table.Application,
			table.MatchAdvances.ApplicationID.EQ(table.Application.ID),
		)

	conditions := []postgres.BoolExpression{whereClause}
	conditions = append(conditions, table.MatchAdvances.EpochIndex.EQ(uint64Expr(epochIndex)))

	tAddr := common.HexToAddress(tournamentAddress)
	conditions = append(conditions, table.MatchAdvances.TournamentAddress.EQ(postgres.Bytea(tAddr.Bytes())))

	idHash := common.HexToHash(idHashHex)
	conditions = append(conditions, table.MatchAdvances.IDHash.EQ(postgres.Bytea(idHash.Bytes())))

	tx, err := beginReadTx(ctx, r.db)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	countStmt := table.MatchAdvances.
		SELECT(postgres.COUNT(postgres.STAR)).
		FROM(fromClause).
		WHERE(postgres.AND(conditions...))

	total, err := countFromTx(ctx, tx, countStmt)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	sel := table.MatchAdvances.
		SELECT(
			table.MatchAdvances.ApplicationID,
			table.MatchAdvances.EpochIndex,
			table.MatchAdvances.TournamentAddress,
			table.MatchAdvances.IDHash,
			table.MatchAdvances.OtherParent,
			table.MatchAdvances.LeftNode,
			table.MatchAdvances.BlockNumber,
			table.MatchAdvances.TxHash,
			table.MatchAdvances.CreatedAt,
			table.MatchAdvances.UpdatedAt,
		).
		FROM(fromClause).
		WHERE(postgres.AND(conditions...))

	if descending {
		sel = sel.ORDER_BY(
			table.MatchAdvances.ApplicationID.DESC(),
			table.MatchAdvances.EpochIndex.DESC(),
			table.MatchAdvances.TournamentAddress.DESC(),
			table.MatchAdvances.IDHash.DESC(),
			table.MatchAdvances.OtherParent.DESC())
	} else {
		sel = sel.ORDER_BY(
			table.MatchAdvances.ApplicationID.ASC(),
			table.MatchAdvances.EpochIndex.ASC(),
			table.MatchAdvances.TournamentAddress.ASC(),
			table.MatchAdvances.IDHash.ASC(),
			table.MatchAdvances.OtherParent.ASC())
	}

	// Apply pagination
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

	var matchAdvances []*model.MatchAdvanced
	for rows.Next() {
		var m model.MatchAdvanced
		err := rows.Scan(
			&m.ApplicationID,
			&m.EpochIndex,
			&m.TournamentAddress,
			&m.IDHash,
			&m.OtherParent,
			&m.LeftNode,
			&m.BlockNumber,
			&m.TxHash,
			&m.CreatedAt,
			&m.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		matchAdvances = append(matchAdvances, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return matchAdvances, total, nil
}
