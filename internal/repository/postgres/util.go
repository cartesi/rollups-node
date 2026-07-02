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

// SubstrBytea returns a substring expression properly typed as ByteaExpression.
// It must render exactly as `substring(col FROM n FOR m)` with inline literals:
// PostgreSQL matches expression indexes structurally (by function OID and
// argument tree), so the schema's `substring(... FROM ... FOR ...)` indexes are
// only usable when the query emits the same function with constant arguments.
// `SUBSTR(col, $1, $2)` is a different catalog function and bypasses them.
func SubstrBytea(col postgres.ColumnBytea, from, count int64) postgres.ByteaExpression {
	qualified := pgx.Identifier{col.TableName(), col.Name()}.Sanitize()
	raw := fmt.Sprintf("substring(%s FROM %d FOR %d)", qualified, from, count)
	return postgres.RawBytea(raw)
}

// ByteaLiteral renders b as an inline bytea literal instead of a bind
// parameter. Inline literals are required where the planner must prove a
// partial-index predicate at plan time (e.g. output_raw_data_address_idx):
// a generic plan cannot prove implication from a parameterized IN list.
func ByteaLiteral(b []byte) postgres.ByteaExpression {
	return postgres.RawBytea(fmt.Sprintf(`'\x%x'::bytea`, b))
}
