package repository

import (
	"context"
	"fmt"
	"math/big"

	. "github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"

	"github.com/jackc/pgx/pgtype"
	"github.com/jackc/pgx/v5"
)

type ComputedClaims struct {
	ID                 uint64
	Hash               Hash
	AppAddress         Address
	IConsensusAddress  Address
	LastBlock         *big.Int
}

func (pg *Database) GetComputedClaims(ctx context.Context) ([]ComputedClaims, error) {
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
		application.iconsensus_address ASC, epoch.index ASC, epoch.id ASC`
	
	args := pgx.NamedArgs{
		"status": EpochStatusClaimComputed,
	}
	rows, err := pg.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}

	var (
		ID                 uint64
		Hash               Hash
		AppAddress         Address
		IConsensusAddress  Address
		lastBlock          pgtype.Numeric
	)
	var data ComputedClaims
	scans := []any{
		&ID,
		&Hash,
		&AppAddress,
		&IConsensusAddress,
		&lastBlock,
	}

	var results []ComputedClaims
	_, err = pgx.ForEachRow(rows, scans, func() error {
		data.ID = ID
		data.Hash = Hash
		data.AppAddress = AppAddress
		data.IConsensusAddress = IConsensusAddress
		if lastBlock.Int == nil { // NOTE: Requires DB to be: Numeric(X,0) NOT NULL
			return fmt.Errorf("Found an invalid block when processing claimID: %v", ID)
		}
		data.LastBlock = lastBlock.Int
		results = append(results, data)
		return nil
	})
	return results, err
}

func (pg *Database) SetClaimAsSubmitted(ctx context.Context, id uint64, transaction_hash common.Hash) (error) {
	var block uint64

	query := `
	UPDATE
		epoch
	SET
		status = @status,
		transaction_hash = @transaction_hash
	WHERE
		epoch.id=@id RETURNING epoch.id`
	
	args := pgx.NamedArgs{
		"id": id,
		"transaction_hash": transaction_hash,
		"status": EpochStatusClaimSubmitted,
	}
	return pg.db.QueryRow(ctx, query, args).Scan(&block)
}
