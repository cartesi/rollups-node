// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"context"
	"fmt"
	"regexp"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

var hexAddressRegex = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

func isHexAddress(s string) bool {
	return hexAddressRegex.MatchString(s)
}

func getWhereClauseFromNameOrAddress(nameOrAddress string) postgres.BoolExpression {
	if isHexAddress(nameOrAddress) {
		address := common.HexToAddress(nameOrAddress)
		return table.Application.IapplicationAddress.EQ(postgres.Bytea(address.Bytes()))
	}
	return table.Application.Name.EQ(postgres.String(nameOrAddress))
}

// uint64Expr converts a uint64 to a go-jet FloatExpression for use with
// PostgreSQL NUMERIC(20,0) "uint64" domain columns.
func uint64Expr(v uint64) postgres.FloatExpression {
	return postgres.RawFloat(fmt.Sprintf("%d", v))
}

func hashToBytes(h *common.Hash) any {
	if h == nil {
		return nil
	}
	return (*h)[:]
}

// beginReadTx starts a REPEATABLE READ, read-only transaction.
// This ensures that multiple queries within the transaction see the same snapshot.
func beginReadTx(ctx context.Context, pool *pgxpool.Pool) (pgx.Tx, error) {
	return pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
}

// countFromTx executes a count query on the given transaction and returns the total.
func countFromTx(ctx context.Context, tx pgx.Tx, countStmt postgres.SelectStatement) (uint64, error) {
	sqlStr, args := countStmt.Sql()
	var total uint64
	err := tx.QueryRow(ctx, sqlStr, args...).Scan(&total)
	return total, err
}

// SubstrBytea returns a SUBSTR expression properly typed as ByteaExpression.
func SubstrBytea(col postgres.ColumnBytea, from, count int64) postgres.ByteaExpression {
	qualified := pgx.Identifier{col.TableName(), col.Name()}.Sanitize()
	raw := fmt.Sprintf("SUBSTR(%s, #from, #count)", qualified)
	return postgres.RawBytea(raw, postgres.RawArgs{"#from": from, "#count": count})
}
