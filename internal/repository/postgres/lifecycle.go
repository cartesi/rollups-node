// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/enum"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

func (r *PostgresRepository) SetApplicationEnabled(ctx context.Context, appID int64, enabled bool) error {
	stmt := table.Application.
		UPDATE(table.Application.Enabled).
		SET(enabled).
		WHERE(
			table.Application.ID.EQ(postgres.Int64(appID)).
				AND(table.Application.DeletedAt.IS_NULL()),
		)

	sqlStr, args := stmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("set application enabled: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

func (r *PostgresRepository) SoftDeleteApplication(ctx context.Context, appID int64) error {
	stmt := table.Application.
		UPDATE(
			table.Application.Enabled,
			table.Application.DeletedAt,
		).
		SET(
			false,
			postgres.NOW(),
		).
		WHERE(
			table.Application.ID.EQ(postgres.Int64(appID)).
				AND(table.Application.DeletedAt.IS_NULL()),
		)

	sqlStr, args := stmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("soft delete application: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

func (r *PostgresRepository) HardDeleteApplication(ctx context.Context, appID int64) error {
	stmt := table.Application.
		DELETE().
		WHERE(
			table.Application.ID.EQ(postgres.Int64(appID)).
				AND(table.Application.DeletedAt.IS_NOT_NULL()),
		)

	sqlStr, args := stmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("hard delete application: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

func (r *PostgresRepository) MarkApplicationRunning(ctx context.Context, appID int64) error {
	stmt := table.Application.
		UPDATE(table.Application.Health).
		SET(enum.ApplicationHealth.Running).
		WHERE(
			table.Application.ID.EQ(postgres.Int64(appID)).
				AND(
					table.Application.Health.IN(
						enum.ApplicationHealth.Stopped,
						enum.ApplicationHealth.Failed,
					),
				),
		)

	sqlStr, args := stmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("mark application running: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

func (r *PostgresRepository) MarkApplicationFailed(
	ctx context.Context, appID int64, reason string,
) error {
	stmt := table.Application.
		UPDATE(
			table.Application.Health,
			table.Application.Reason,
		).
		SET(
			enum.ApplicationHealth.Failed,
			reason,
		).
		WHERE(
			table.Application.ID.EQ(postgres.Int64(appID)).
				AND(table.Application.Health.EQ(enum.ApplicationHealth.Running)),
		)

	sqlStr, args := stmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("mark application failed: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

func (r *PostgresRepository) MarkApplicationInoperable(
	ctx context.Context, appID int64, reason string,
) error {
	stmt := table.Application.
		UPDATE(
			table.Application.Health,
			table.Application.Reason,
		).
		SET(
			enum.ApplicationHealth.Inoperable,
			reason,
		).
		WHERE(
			table.Application.ID.EQ(postgres.Int64(appID)).
				AND(table.Application.Health.EQ(enum.ApplicationHealth.Running)),
		)

	sqlStr, args := stmt.Sql()
	cmd, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("mark application inoperable: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

func (r *PostgresRepository) AcknowledgeAppStopped(
	ctx context.Context, appID int64, serviceName string,
) error {
	stmt := table.ApplicationServiceAck.
		INSERT(
			table.ApplicationServiceAck.ApplicationID,
			table.ApplicationServiceAck.ServiceName,
		).
		VALUES(
			appID,
			serviceName,
		).
		ON_CONFLICT(
			table.ApplicationServiceAck.ApplicationID,
			table.ApplicationServiceAck.ServiceName,
		).
		DO_NOTHING()

	sqlStr, args := stmt.Sql()
	_, err := r.db.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("acknowledge app stopped: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetPendingAcks(
	ctx context.Context, appID int64, requiredServices []string,
) ([]string, error) {
	stmt := table.ApplicationServiceAck.
		SELECT(table.ApplicationServiceAck.ServiceName).
		WHERE(table.ApplicationServiceAck.ApplicationID.EQ(postgres.Int64(appID)))

	sqlStr, args := stmt.Sql()
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("get pending acks: %w", err)
	}
	defer rows.Close()

	acked := make(map[string]struct{}, len(requiredServices))
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		acked[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	pending := make([]string, 0, len(requiredServices))
	for _, serviceName := range requiredServices {
		if _, ok := acked[serviceName]; !ok {
			pending = append(pending, serviceName)
		}
	}
	return pending, nil
}

func (r *PostgresRepository) GetAppsNeedingAck(
	ctx context.Context, serviceName string, consensusTypes []Consensus,
) ([]int64, error) {
	if len(consensusTypes) == 0 {
		return nil, nil
	}

	// Build consensus type filter expressions.
	ctExprs := make([]postgres.Expression, len(consensusTypes))
	for i, ct := range consensusTypes {
		ctExprs[i] = postgres.NewEnumValue(string(ct))
	}

	// Anti-join: find soft-deleted apps (of matching consensus types) that
	// this service has not yet acked.
	stmt := table.Application.
		SELECT(table.Application.ID).
		FROM(
			table.Application.LEFT_JOIN(
				table.ApplicationServiceAck,
				table.ApplicationServiceAck.ApplicationID.EQ(table.Application.ID).
					AND(table.ApplicationServiceAck.ServiceName.EQ(postgres.String(serviceName))),
			),
		).
		WHERE(
			table.ApplicationServiceAck.ApplicationID.IS_NULL().
				AND(table.Application.DeletedAt.IS_NOT_NULL()).
				AND(table.Application.ConsensusType.IN(ctExprs...)),
		)

	sqlStr, args := stmt.Sql()
	rows, err := r.db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("get apps needing ack: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
