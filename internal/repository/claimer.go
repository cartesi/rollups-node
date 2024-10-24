package repository

import (
	"context"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type ComputedClaim struct {
	Hash                 common.Hash
	EpochID              uint64
	AppContractAddress   Address
	AppIConsensusAddress Address
	EpochLastBlock       uint64
}

func (pg *Database) SelectComputedClaims(ctx context.Context) ([]ComputedClaim, error) {
	query := `
	SELECT
		epoch.id,
		epoch.claim_hash,
		application.contract_address,
		application.iconsensus_address,
		epoch.last_block
	FROM
		epoch
	INNER JOIN
		application
	ON
		epoch.application_address = application.contract_address
	WHERE
		epoch.status = @status
	ORDER BY
		epoch.application_address ASC, epoch.index ASC`
	
	args := pgx.NamedArgs{
		"status": EpochStatusClaimComputed,
	}
	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}

	var data ComputedClaim
	scans := []any{
		&data.EpochID,
		&data.Hash,
		&data.AppContractAddress,
		&data.AppIConsensusAddress,
		&data.EpochLastBlock,
	}

	var results []ComputedClaim
	_, err = pgx.ForEachRow(rows, scans, func() error {
		results = append(results, data)
		return nil
	})
	return results, err
}

func (pg *Database) UpdateEpochWithSubmittedClaim(
	ctx context.Context,
	id uint64,
	transaction_hash common.Hash,
) (pgconn.CommandTag, error) {
	query := `
	UPDATE
		epoch
	SET
		status = @status,
		transaction_hash = @transaction_hash
	WHERE
		status = @prevStatus AND epoch.id = @id`
	
	args := pgx.NamedArgs{
		"id": id,
		"transaction_hash": transaction_hash,
		"status": EpochStatusClaimSubmitted,
		"prevStatus": EpochStatusClaimComputed,
	}
	return pg.db.Exec(ctx, query, args)
}
