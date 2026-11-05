// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/jsonutil"
	"github.com/cartesi/rollups-node/internal/model"
)

func TestApplicationDeploymentReportKeepsTransaction(t *testing.T) {
	report := applicationDeploymentReport{
		applicationDeploymentFields: applicationDeploymentFields(model.Application{Name: "app", EpochLength: 17}),
		Transaction:                 cli.TransactionResult{},
	}
	for _, value := range []any{report, &report} {
		data, err := json.Marshal(value, jsonutil.Options)
		require.NoError(t, err)
		var fields map[string]jsontext.Value
		require.NoError(t, json.Unmarshal(data, &fields, jsonutil.Options))
		require.Equal(t, `"app"`, string(fields["name"]))
		require.Equal(t, "17", string(fields["epoch_length"]))
		require.Contains(t, fields, "transaction")
	}
}
