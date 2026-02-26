// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"fmt"
	"regexp"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/jackc/pgx/v5"

	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
)

var hexAddressRegex = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

func isHexAddress(s string) bool {
	return hexAddressRegex.MatchString(s)
}

func getWhereClauseFromNameOrAddress(nameOrAddress string) (postgres.BoolExpression, error) {

	var whereClause postgres.BoolExpression
	if isHexAddress(nameOrAddress) {
		address := common.HexToAddress(nameOrAddress)
		whereClause = table.Application.IapplicationAddress.EQ(postgres.Bytea(address.Bytes()))
	} else {
		// treat as name
		whereClause = table.Application.Name.EQ(postgres.String(nameOrAddress))
	}
	return whereClause, nil
}

func hashToBytes(h *common.Hash) any {
	if h == nil {
		return nil
	}
	return (*h)[:]
}

func addressToBytes(a *common.Address) any {
	if a == nil {
		return nil
	}
	return (*a)[:]
}

// SubstrBytea returns a SUBSTR expression properly typed as ByteaExpression.
func SubstrBytea(col postgres.ColumnBytea, from, count int64) postgres.ByteaExpression {
	qualified := pgx.Identifier{col.TableName(), col.Name()}.Sanitize()
	raw := fmt.Sprintf("SUBSTR(%s, #from, #count)", qualified)
	return postgres.RawBytea(raw, postgres.RawArgs{"#from": from, "#count": count})
}
