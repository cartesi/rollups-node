// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"regexp"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-jet/jet/v2/postgres"

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
