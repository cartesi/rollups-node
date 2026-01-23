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

// ------------------------ CommitmentRepository Methods ------------------------ //

func (r *PostgresRepository) CreateCommitment(
	ctx context.Context,
	nameOrAddress string,
	c *model.Commitment,
) error {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return err
	}

	selectQuery := table.Application.SELECT(
		table.Application.ID,
		postgres.RawFloat(fmt.Sprintf("%d", c.EpochIndex)),
		postgres.Bytea(c.TournamentAddress.Bytes()),
		postgres.Bytea(c.Commitment.Bytes()),
		postgres.Bytea(c.FinalStateHash.Bytes()),
		postgres.Bytea(c.SubmitterAddress.Bytes()),
		postgres.RawFloat(fmt.Sprintf("%d", c.BlockNumber)),
		postgres.Bytea(c.TxHash.Bytes()),
	).WHERE(
		whereClause,
	)

	insertStmt := table.Commitments.INSERT(
		table.Commitments.ApplicationID,
		table.Commitments.EpochIndex,
		table.Commitments.TournamentAddress,
		table.Commitments.Commitment,
		table.Commitments.FinalStateHash,
		table.Commitments.SubmitterAddress,
		table.Commitments.BlockNumber,
		table.Commitments.TxHash,
	).QUERY(
		selectQuery,
	)

	sqlStr, args := insertStmt.Sql()
	_, err = r.db.Exec(ctx, sqlStr, args...)

	return err
}

func (r *PostgresRepository) GetCommitment(
	ctx context.Context,
	nameOrAddress string,
	epochIndex uint64,
	tournamentAddress string,
	commitmentHex string,
) (*model.Commitment, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, err
	}

	tournamentAddr := common.HexToAddress(tournamentAddress)
	commitment := common.HexToHash(commitmentHex)

	sel := table.Commitments.
		SELECT(
			table.Commitments.ApplicationID,
			table.Commitments.EpochIndex,
			table.Commitments.TournamentAddress,
			table.Commitments.Commitment,
			table.Commitments.FinalStateHash,
			table.Commitments.SubmitterAddress,
			table.Commitments.BlockNumber,
			table.Commitments.TxHash,
			table.Commitments.CreatedAt,
			table.Commitments.UpdatedAt,
		).
		FROM(
			table.Commitments.
				INNER_JOIN(table.Application,
					table.Commitments.ApplicationID.EQ(table.Application.ID),
				),
		).
		WHERE(
			whereClause.
				AND(table.Commitments.EpochIndex.EQ(postgres.RawFloat(fmt.Sprintf("%d", epochIndex)))).
				AND(table.Commitments.TournamentAddress.EQ(postgres.Bytea(tournamentAddr.Bytes()))).
				AND(table.Commitments.Commitment.EQ(postgres.Bytea(commitment.Bytes()))),
		)

	sqlStr, args := sel.Sql()
	row := r.db.QueryRow(ctx, sqlStr, args...)

	var c model.Commitment
	err = row.Scan(
		&c.ApplicationID,
		&c.EpochIndex,
		&c.TournamentAddress,
		&c.Commitment,
		&c.FinalStateHash,
		&c.SubmitterAddress,
		&c.BlockNumber,
		&c.TxHash,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *PostgresRepository) ListCommitments(
	ctx context.Context,
	nameOrAddress string,
	f repository.CommitmentFilter,
	p repository.Pagination,
	descending bool,
) ([]*model.Commitment, uint64, error) {

	whereClause, err := getWhereClauseFromNameOrAddress(nameOrAddress)
	if err != nil {
		return nil, 0, err
	}

	sel := table.Commitments.
		SELECT(
			table.Commitments.ApplicationID,
			table.Commitments.EpochIndex,
			table.Commitments.TournamentAddress,
			table.Commitments.Commitment,
			table.Commitments.FinalStateHash,
			table.Commitments.SubmitterAddress,
			table.Commitments.BlockNumber,
			table.Commitments.TxHash,
			table.Commitments.CreatedAt,
			table.Commitments.UpdatedAt,
			postgres.COUNT(postgres.STAR).OVER().AS("total_count"),
		).
		FROM(
			table.Commitments.
				INNER_JOIN(table.Application,
					table.Commitments.ApplicationID.EQ(table.Application.ID),
				),
		)

	conditions := []postgres.BoolExpression{whereClause}
	if f.EpochIndex != nil {
		conditions = append(conditions, table.Commitments.EpochIndex.EQ(postgres.RawFloat(fmt.Sprintf("%d", *f.EpochIndex))))
	}
	if f.TournamentAddress != nil {
		tournamentAddr := common.HexToAddress(*f.TournamentAddress)
		conditions = append(conditions, table.Commitments.TournamentAddress.EQ(postgres.Bytea(tournamentAddr.Bytes())))
	}

	sel = sel.WHERE(postgres.AND(conditions...))

	if descending {
		sel = sel.ORDER_BY(
			table.Commitments.ApplicationID.DESC(),
			table.Commitments.EpochIndex.DESC(),
			table.Commitments.TournamentAddress.DESC(),
			table.Commitments.Commitment.DESC())
	} else {
		sel = sel.ORDER_BY(
			table.Commitments.ApplicationID.ASC(),
			table.Commitments.EpochIndex.ASC(),
			table.Commitments.TournamentAddress.ASC(),
			table.Commitments.Commitment.ASC())
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

	var commitments []*model.Commitment
	var total uint64
	for rows.Next() {
		var c model.Commitment
		err := rows.Scan(
			&c.ApplicationID,
			&c.EpochIndex,
			&c.TournamentAddress,
			&c.Commitment,
			&c.FinalStateHash,
			&c.SubmitterAddress,
			&c.BlockNumber,
			&c.TxHash,
			&c.CreatedAt,
			&c.UpdatedAt,
			&total,
		)
		if err != nil {
			return nil, 0, err
		}
		commitments = append(commitments, &c)
	}

	return commitments, total, nil
}
