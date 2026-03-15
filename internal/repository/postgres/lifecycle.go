// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/repository"
)

func (r *PostgresRepository) SetApplicationEnabled(ctx context.Context, appID int64, enabled bool) error {
	cmd, err := r.db.Exec(ctx,
		`UPDATE application SET enabled = $2 WHERE id = $1 AND deleted_at IS NULL`,
		appID, enabled)
	if err != nil {
		return fmt.Errorf("set application enabled: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

func (r *PostgresRepository) SoftDeleteApplication(ctx context.Context, appID int64) error {
	cmd, err := r.db.Exec(ctx,
		`UPDATE application SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`,
		appID)
	if err != nil {
		return fmt.Errorf("soft delete application: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

func (r *PostgresRepository) HardDeleteApplication(ctx context.Context, appID int64) error {
	cmd, err := r.db.Exec(ctx,
		`DELETE FROM application WHERE id = $1`,
		appID)
	if err != nil {
		return fmt.Errorf("hard delete application: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

func (r *PostgresRepository) MarkApplicationRunning(ctx context.Context, appID int64) error {
	cmd, err := r.db.Exec(ctx,
		`UPDATE application SET health = 'RUNNING' WHERE id = $1 AND health IN ('STOPPED', 'FAILED')`,
		appID)
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
	cmd, err := r.db.Exec(ctx,
		`UPDATE application SET health = 'FAILED', reason = $2 WHERE id = $1 AND health = 'RUNNING'`,
		appID, reason)
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
	cmd, err := r.db.Exec(ctx,
		`UPDATE application SET health = 'INOPERABLE', reason = $2
		 WHERE id = $1 AND health = 'RUNNING'`,
		appID, reason)
	if err != nil {
		return fmt.Errorf("mark application inoperable: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

func (r *PostgresRepository) MarkApplicationStopped(ctx context.Context, appID int64) error {
	cmd, err := r.db.Exec(ctx,
		`UPDATE application SET health = 'STOPPED' WHERE id = $1 AND health = 'RUNNING'`,
		appID)
	if err != nil {
		return fmt.Errorf("mark application stopped: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNoUpdate
	}
	return nil
}

func (r *PostgresRepository) AcknowledgeAppStopped(
	ctx context.Context, appID int64, serviceName string,
) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO application_service_ack (application_id, service_name)
		 VALUES ($1, $2)
		 ON CONFLICT (application_id, service_name) DO NOTHING`,
		appID, serviceName)
	if err != nil {
		return fmt.Errorf("acknowledge app stopped: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetPendingAcks(
	ctx context.Context, appID int64, requiredServices []string,
) ([]string, error) {
	rows, err := r.db.Query(ctx,
		`SELECT s.name FROM unnest($2::text[]) AS s(name)
		 WHERE s.name NOT IN (
		     SELECT service_name FROM application_service_ack WHERE application_id = $1
		 )`,
		appID, requiredServices)
	if err != nil {
		return nil, fmt.Errorf("get pending acks: %w", err)
	}
	defer rows.Close()

	var pending []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		pending = append(pending, name)
	}
	return pending, rows.Err()
}

func (r *PostgresRepository) ClearAcks(ctx context.Context, appID int64) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM application_service_ack WHERE application_id = $1`,
		appID)
	if err != nil {
		return fmt.Errorf("clear acks: %w", err)
	}
	return nil
}
