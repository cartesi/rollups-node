// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package config

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestPersistentConfigRejectsInvalidJSONFields(t *testing.T) {
	for _, raw := range []string{
		`null`, `{}`, `[]`, ``, `{`,
		`{"DefaultBlock":"FINALIZED"}`,
		`{"ChainID":1}`,
		`{"DefaultBlock":null,"ChainID":1}`,
		`{"DefaultBlock":"FINALIZED","ChainID":null}`,
		`{"DefaultBlock":"FINALIZED","ChainID":0}`,
		`{"DefaultBlock":"FINALIZED","ChainID":-1}`,
		`{"DefaultBlock":"FINALIZED","ChainID":18446744073709551616}`,
		`{"DefaultBlock":"FINALIZED","ChainID":"1"}`,
		`{"DefaultBlock":1,"ChainID":1}`,
		`{"DefaultBlock":"finalized","ChainID":1}`,
		`{"DefaultBlock":"INVALID","ChainID":1}`,
	} {
		t.Run(raw, func(t *testing.T) {
			var chain PersistentChainConfig
			require.Error(t, json.Unmarshal([]byte(raw), &chain))
			var submitter PersistentSubmitterConfig
			require.Error(t, json.Unmarshal([]byte(raw), &submitter))
		})
	}
	for _, mode := range []string{"", `,"ClaimSubmissionEnabled":null`, `,"ClaimSubmissionEnabled":"false"`} {
		t.Run("mode"+mode, func(t *testing.T) {
			raw := []byte(`{"DefaultBlock":"FINALIZED","ChainID":1` + mode + `}`)
			var submitter PersistentSubmitterConfig
			require.Error(t, json.Unmarshal(raw, &submitter))
		})
	}
}

func TestPersistentConfigRoundTrip(t *testing.T) {
	for _, policy := range model.DefaultBlockAllValues {
		for _, enabled := range []bool{false, true} {
			want := PersistentSubmitterConfig{
				DefaultBlock: policy, ClaimSubmissionEnabled: enabled, ChainID: math.MaxUint64,
			}
			if enabled {
				want.ResolvedSignerAddress = common.HexToAddress("0x1")
			}
			raw, err := json.Marshal(want)
			require.NoError(t, err)
			var got PersistentSubmitterConfig
			require.NoError(t, json.Unmarshal(raw, &got))
			require.Equal(t, want, got)
			require.NoError(t, got.CheckRequested(want))
			var chain PersistentChainConfig
			require.NoError(t, json.Unmarshal(raw, &chain))
			require.NoError(t, chain.CheckRequested(want.chainConfig()))
		}
	}
}

func TestPersistentConfigAcceptsUnknownFields(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"DefaultBlock":"FINALIZED","ChainID":42,"ClaimSubmissionEnabled":true,
        "ResolvedSignerAddress":"0x0000000000000000000000000000000000000001","ExtraSetting":{"enabled":false}}`)
	var chain PersistentChainConfig
	require.NoError(t, json.Unmarshal(raw, &chain))
	require.Equal(t, PersistentChainConfig{DefaultBlock: model.DefaultBlock_Finalized, ChainID: 42}, chain)
	var submitter PersistentSubmitterConfig
	require.NoError(t, json.Unmarshal(raw, &submitter))
	require.NoError(t, submitter.CheckRequested(PersistentSubmitterConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: 42, ClaimSubmissionEnabled: true,
		ResolvedSignerAddress: common.HexToAddress("0x1"),
	}))
	roundTrip, err := json.Marshal(submitter)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(roundTrip, &fields))
	require.NotContains(t, fields, "ExtraSetting")
}

func TestPersistentConfigRejectsRequestedChanges(t *testing.T) {
	saved := PersistentSubmitterConfig{DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1}
	for _, test := range []struct {
		name   string
		change func(*PersistentSubmitterConfig)
		want   string
	}{
		{"chain", func(c *PersistentSubmitterConfig) { c.ChainID = 2 }, "database=1, configured=2"},
		{"policy", func(c *PersistentSubmitterConfig) { c.DefaultBlock = model.DefaultBlock_Latest },
			"database=FINALIZED, configured=LATEST"},
		{"mode", func(c *PersistentSubmitterConfig) { c.ClaimSubmissionEnabled = true }, "database=false, configured=true"},
		{"invalid chain", func(c *PersistentSubmitterConfig) { c.ChainID = 0 }, "invalid requested config"},
		{"invalid policy", func(c *PersistentSubmitterConfig) { c.DefaultBlock = "" }, "invalid requested config"},
	} {
		t.Run(test.name, func(t *testing.T) {
			requested := saved
			test.change(&requested)
			require.ErrorContains(t, saved.CheckRequested(requested), test.want)
		})
	}
	requested := saved
	saved.ClaimSubmissionEnabled = true
	require.ErrorContains(t, saved.CheckRequested(requested), "database=true, configured=false")
	saved.ChainID = 0
	require.ErrorContains(t, saved.CheckRequested(requested), "invalid saved config")
}

func TestCheckNetworkChainID(t *testing.T) {
	for _, invalid := range []*big.Int{nil, big.NewInt(0), big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 64)} {
		require.ErrorContains(t, CheckNetworkChainID(invalid, 1), "invalid network chain ID")
	}
	require.ErrorContains(t, CheckNetworkChainID(big.NewInt(2), 1), "network=2, configured=1")
	require.NoError(t, CheckNetworkChainID(new(big.Int).SetUint64(math.MaxUint64), math.MaxUint64))
}

func TestPersistentSubmitterIdentityMatchesMode(t *testing.T) {
	for _, test := range []struct {
		name     string
		enabled  bool
		identity string
		valid    bool
	}{
		{"observer without key", false, "", true},
		{"observer zero key", false, `,"ResolvedSignerAddress":"0x0000000000000000000000000000000000000000"`, true},
		{"observer with key", false, `,"ResolvedSignerAddress":"0x0000000000000000000000000000000000000001"`, false},
		{"submitter without key", true, "", false},
		{"submitter null key", true, `,"ResolvedSignerAddress":null`, false},
		{"submitter zero key", true, `,"ResolvedSignerAddress":"0x0000000000000000000000000000000000000000"`, false},
		{"submitter malformed key", true, `,"ResolvedSignerAddress":"0x01"`, false},
		{"submitter fixed key", true, `,"ResolvedSignerAddress":"0x0000000000000000000000000000000000000001"`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"DefaultBlock":"FINALIZED","ChainID":1,"ClaimSubmissionEnabled":%t%s}`,
				test.enabled, test.identity)
			var value PersistentSubmitterConfig
			err := json.Unmarshal([]byte(raw), &value)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestPersistentSubmitterRejectsSignerChange(t *testing.T) {
	saved := PersistentSubmitterConfig{
		DefaultBlock: model.DefaultBlock_Finalized, ChainID: 1, ClaimSubmissionEnabled: true,
		ResolvedSignerAddress: common.HexToAddress("0x1"),
	}
	requested := saved
	requested.ResolvedSignerAddress = common.HexToAddress("0x2")
	require.ErrorContains(t, saved.CheckRequested(requested), "resolved signer address mismatch")
}
