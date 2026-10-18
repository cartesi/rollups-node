// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"encoding/json"
	"testing"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/postgres/db/rollupsdb/public/table"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/go-jet/jet/v2/postgres"
	"github.com/stretchr/testify/require"
)

func TestDeliveryFixedRegistrationPreservesUnknownSettings(t *testing.T) {
	r := newEpochPublicationRepository(t).(*PostgresRepository)
	signer := repotest.UniqueAddress()
	registerDeliveryTest(t, r, signer)
	raw, _, _, err := r.LoadNodeConfigRaw(t.Context(), deliveryPRTKey)
	require.NoError(t, err)
	var settings map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &settings))
	settings["FutureField"] = json.RawMessage(`{"value":42}`)
	raw, err = json.Marshal(settings)
	require.NoError(t, err)
	stmt := table.NodeConfig.UPDATE(table.NodeConfig.Value).SET(postgres.Json(raw)).
		WHERE(table.NodeConfig.Key.EQ(postgres.String(deliveryPRTKey)))
	query, args := stmt.Sql()
	_, err = r.db.Exec(t.Context(), query, args...)
	require.NoError(t, err)
	requested := config.PersistentSubmitterConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1,
		ClaimSubmissionEnabled: true, ResolvedSignerAddress: signer,
	}
	_, err = r.RegisterSubmitter(t.Context(), deliveryPRTKey, requested)
	require.NoError(t, err)
	raw, _, _, err = r.LoadNodeConfigRaw(t.Context(), deliveryPRTKey)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	require.JSONEq(t, `{"value":42}`, string(fields["FutureField"]))
	requested.ResolvedSignerAddress = repotest.UniqueAddress()
	_, err = r.RegisterSubmitter(t.Context(), deliveryPRTKey, requested)
	require.ErrorContains(t, err, "resolved signer address mismatch")
}
