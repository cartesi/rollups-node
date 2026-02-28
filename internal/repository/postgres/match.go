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

// ------------------------ MatchRepository Methods ------------------------ //

func (r *PostgresRepository) CreateMatch(
	ctx context.Context,
	nameOrAddress string,
	m *model.Match,
) error {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	selectQuery := table.Application.SELECT(
		table.Application.ID,
		uint64Expr(m.EpochIndex),
		postgres.Bytea(m.TournamentAddress.Bytes()),
		postgres.Bytea(m.IDHash.Bytes()),
		postgres.Bytea(m.CommitmentOne.Bytes()),
		postgres.Bytea(m.CommitmentTwo.Bytes()),
		postgres.Bytea(m.LeftOfTwo.Bytes()),
		uint64Expr(m.BlockNumber),
		postgres.Bytea(m.TxHash.Bytes()),
		postgres.NewEnumValue(m.Winner.String()),
		postgres.NewEnumValue(m.DeletionReason.String()),
		uint64Expr(m.DeletionBlockNumber),
		postgres.Bytea(m.DeletionTxHash.Bytes()),
	).WHERE(
		whereClause,
	)

	insertStmt := table.Matches.INSERT(
		table.Matches.ApplicationID,
		table.Matches.EpochIndex,
		table.Matches.TournamentAddress,
		table.Matches.IDHash,
		table.Matches.CommitmentOne,
		table.Matches.CommitmentTwo,
		table.Matches.LeftOfTwo,
		table.Matches.BlockNumber,
		table.Matches.TxHash,
		table.Matches.Winner,
		table.Matches.DeletionReason,
		table.Matches.DeletionBlockNumber,
		table.Matches.DeletionTxHash,
	).QUERY(
		selectQuery,
	)

	sqlStr, args := insertStmt.Sql()
	_, err := r.db.Exec(ctx, sqlStr, args...)

	return err
}

func (r *PostgresRepository) UpdateMatch(
	ctx context.Context,
	nameOrAddress string,
	m *model.Match,
) error {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	updateStmt := table.Matches.
		UPDATE(
			table.Matches.Winner,
			table.Matches.DeletionReason,
			table.Matches.DeletionBlockNumber,
			table.Matches.DeletionTxHash,
		).
		SET(
			m.Winner,
			m.DeletionReason,
			m.DeletionBlockNumber,
			postgres.Bytea(m.DeletionTxHash.Bytes()),
		).
		FROM(
			table.Application,
		).
		WHERE(
			whereClause.
				AND(table.Matches.ApplicationID.EQ(postgres.Int(m.ApplicationID))).
				AND(table.Matches.EpochIndex.EQ(uint64Expr(m.EpochIndex))).
				AND(table.Matches.TournamentAddress.EQ(postgres.Bytea(m.TournamentAddress.Bytes()))).
				AND(table.Matches.IDHash.EQ(postgres.Bytea(m.IDHash.Bytes()))),
		)

	sqlStr, args := updateStmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) GetMatch(
	ctx context.Context,
	nameOrAddress string,
	epochIndex uint64,
	tournamentAddress string,
	idHashHex string,
) (*model.Match, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	tournamentAddr := common.HexToAddress(tournamentAddress)
	idHash := common.HexToHash(idHashHex)

	sel := table.Matches.
		SELECT(
			table.Matches.ApplicationID,
			table.Matches.EpochIndex,
			table.Matches.TournamentAddress,
			table.Matches.IDHash,
			table.Matches.CommitmentOne,
			table.Matches.CommitmentTwo,
			table.Matches.LeftOfTwo,
			table.Matches.BlockNumber,
			table.Matches.TxHash,
			table.Matches.Winner,
			table.Matches.DeletionReason,
			table.Matches.DeletionBlockNumber,
			table.Matches.DeletionTxHash,
			table.Matches.CreatedAt,
			table.Matches.UpdatedAt,
		).
		FROM(
			table.Matches.
				INNER_JOIN(table.Application,
					table.Matches.ApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.Matches.EpochIndex.EQ(uint64Expr(epochIndex))).
				AND(table.Matches.TournamentAddress.EQ(postgres.Bytea(tournamentAddr.Bytes()))).
				AND(table.Matches.IDHash.EQ(postgres.Bytea(idHash.Bytes()))),
		)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var m model.Match
	err := row.Scan(
		&m.ApplicationID,
		&m.EpochIndex,
		&m.TournamentAddress,
		&m.IDHash,
		&m.CommitmentOne,
		&m.CommitmentTwo,
		&m.LeftOfTwo,
		&m.BlockNumber,
		&m.TxHash,
		&m.Winner,
		&m.DeletionReason,
		&m.DeletionBlockNumber,
		&m.DeletionTxHash,
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

func (r *PostgresRepository) ListMatches(
	ctx context.Context,
	nameOrAddress string,
	f repository.MatchFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Match, uint64, error) {

	whereClause := getWhereClauseFromNameOrAddress(nameOrAddress)

	sel := table.Matches.
		SELECT(
			table.Matches.ApplicationID,
			table.Matches.EpochIndex,
			table.Matches.TournamentAddress,
			table.Matches.IDHash,
			table.Matches.CommitmentOne,
			table.Matches.CommitmentTwo,
			table.Matches.LeftOfTwo,
			table.Matches.BlockNumber,
			table.Matches.TxHash,
			table.Matches.Winner,
			table.Matches.DeletionReason,
			table.Matches.DeletionBlockNumber,
			table.Matches.DeletionTxHash,
			table.Matches.CreatedAt,
			table.Matches.UpdatedAt,
			postgres.COUNT(postgres.STAR).OVER().AS("total_count"),
		).
		FROM(
			table.Matches.
				INNER_JOIN(table.Application,
					table.Matches.ApplicationID.EQ(table.Application.ID),
				),
		)

	conditions := []postgres.BoolExpression{whereClause}
	if f.EpochIndex != nil {
		conditions = append(conditions, table.Matches.EpochIndex.EQ(uint64Expr(*f.EpochIndex)))
	}
	if f.TournamentAddress != nil {
		tournamentAddr := common.HexToAddress(*f.TournamentAddress)
		conditions = append(conditions, table.Matches.TournamentAddress.EQ(postgres.Bytea(tournamentAddr.Bytes())))
	}

	sel = sel.WHERE(postgres.AND(conditions...))

	if descending {
		sel = sel.ORDER_BY(table.Matches.EpochIndex.DESC())
	} else {
		sel = sel.ORDER_BY(table.Matches.EpochIndex.ASC())
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

	var matches []*model.Match
	var total uint64
	for rows.Next() {
		var m model.Match
		err := rows.Scan(
			&m.ApplicationID,
			&m.EpochIndex,
			&m.TournamentAddress,
			&m.IDHash,
			&m.CommitmentOne,
			&m.CommitmentTwo,
			&m.LeftOfTwo,
			&m.BlockNumber,
			&m.TxHash,
			&m.Winner,
			&m.DeletionReason,
			&m.DeletionBlockNumber,
			&m.DeletionTxHash,
			&m.CreatedAt,
			&m.UpdatedAt,
			&total,
		)
		if err != nil {
			return nil, 0, err
		}
		matches = append(matches, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return matches, total, nil
}
