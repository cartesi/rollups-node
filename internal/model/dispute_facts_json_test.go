// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestDisputeFactsJSONQuantities(t *testing.T) {
	const largeHex = "0x100000000000000000000000000000001"
	const maximumHex = "0xffffffffffffffff"
	var large Uint256
	require.NoError(t, json.Unmarshal([]byte(`"0x100000000000000000000000000000001"`), &large))
	zero := common.Hash{}
	for _, test := range []struct {
		name    string
		value   any
		decoded any
		want    map[string]string
	}{
		{
			name:    "tournament",
			value:   Tournament{BondValue: large, ContestedFinalStateOne: &zero, ContestedFinalStateTwo: &zero},
			decoded: new(Tournament),
			want: map[string]string{
				"bond_value": largeHex, "contested_final_state_one": zero.Hex(), "contested_final_state_two": zero.Hex(),
			},
		},
		{
			name:  "match",
			value: Match{ResponderDeadline: math.MaxUint64}, decoded: new(Match),
			want: map[string]string{"responder_deadline": maximumHex},
		},
		{
			name:  "advance facts",
			value: MatchAdvanced{CurrentHeight: math.MaxUint64, ResponderDeadline: math.MaxUint64}, decoded: new(MatchAdvanced),
			want: map[string]string{"current_height": maximumHex, "responder_deadline": maximumHex},
		},
		{
			name: "seal",
			value: LeafMatchSeal{
				AgreeState: zero, DivergencePosition: large, FinalStateOne: zero, FinalStateTwo: zero,
				DeadlineOne: math.MaxUint64, DeadlineTwo: math.MaxUint64,
			},
			decoded: new(LeafMatchSeal),
			want: map[string]string{
				"agree_state": zero.Hex(), "divergence_position": largeHex,
				"final_state_one": zero.Hex(), "final_state_two": zero.Hex(),
				"deadline_one": maximumHex, "deadline_two": maximumHex,
			},
		},
		{
			name: "standing",
			value: StandingChanged{
				EpochIndex: math.MaxUint64, MatchCount: large, ResultAt: math.MaxUint64,
				WinnerExpiresAt: math.MaxUint64, BlockNumber: math.MaxUint64, LogIndex: math.MaxUint64,
			},
			decoded: new(StandingChanged),
			want: map[string]string{
				"epoch_index": maximumHex, "match_count": largeHex, "result_at": maximumHex,
				"winner_expires_at": maximumHex, "block_number": maximumHex, "log_index": maximumHex,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(test.value)
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(data, &fields))
			for field, want := range test.want {
				require.JSONEq(t, `"`+want+`"`, string(fields[field]), field)
			}
			require.NoError(t, json.Unmarshal(data, test.decoded))
			reencoded, err := json.Marshal(test.decoded)
			require.NoError(t, err)
			require.JSONEq(t, string(data), string(reencoded))
		})
	}
}

func TestDisputeFactsJSONHashPresence(t *testing.T) {
	var root Tournament
	require.NoError(t, json.Unmarshal([]byte(`{"contested_final_state_one":null,"contested_final_state_two":null}`), &root))
	require.Nil(t, root.ContestedFinalStateOne)
	require.Nil(t, root.ContestedFinalStateTwo)

	var standing StandingChanged
	require.NoError(t, json.Unmarshal([]byte(`{"dangling":null,"parent_commitment":null}`), &standing))
	require.Nil(t, standing.Dangling)
	require.Nil(t, standing.ParentCommitment)

	data, err := json.Marshal(standing)
	require.NoError(t, err)
	require.Contains(t, string(data), `"dangling":null`)
	require.Contains(t, string(data), `"parent_commitment":null`)
}
