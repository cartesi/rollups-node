// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListOutputsParamsStringOrList(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected StringOrList
	}{
		"single selector": {
			input:    `{"output_type":"0x237a816f"}`,
			expected: StringOrList{"0x237a816f"},
		},
		"selector list": {
			input:    `{"output_type":["0x237a816f","0x10321e8b"]}`,
			expected: StringOrList{"0x237a816f", "0x10321e8b"},
		},
		"empty list": {
			input:    `{"output_type":[]}`,
			expected: StringOrList{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var params ListOutputsParams
			require.NoError(t, json.Unmarshal([]byte(test.input), &params))
			require.NotNil(t, params.OutputType)
			require.Equal(t, test.expected, *params.OutputType)
		})
	}
}

func TestListEpochsParamsStringOrList(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected StringOrList
	}{
		"single status": {
			input:    `{"status":"OPEN"}`,
			expected: StringOrList{"OPEN"},
		},
		"status list": {
			input:    `{"status":["OPEN","CLOSED"]}`,
			expected: StringOrList{"OPEN", "CLOSED"},
		},
		"empty list": {
			input:    `{"status":[]}`,
			expected: StringOrList{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var params ListEpochsParams
			require.NoError(t, json.Unmarshal([]byte(test.input), &params))
			require.NotNil(t, params.Status)
			require.Equal(t, test.expected, *params.Status)
		})
	}
}

func TestListOutputsParamsExecutedIsOptional(t *testing.T) {
	var omitted ListOutputsParams
	require.NoError(t, json.Unmarshal([]byte(`{}`), &omitted))
	require.Nil(t, omitted.Executed)

	var executed ListOutputsParams
	require.NoError(t, json.Unmarshal([]byte(`{"executed":true}`), &executed))
	require.NotNil(t, executed.Executed)
	require.True(t, *executed.Executed)

	var pending ListOutputsParams
	require.NoError(t, json.Unmarshal([]byte(`{"executed":false}`), &pending))
	require.NotNil(t, pending.Executed)
	require.False(t, *pending.Executed)
}

func TestPositionalParamsDeclarationOrder(t *testing.T) {
	tests := map[string]struct {
		newTarget  func() any
		positional string
		named      string
	}{
		"ListApplicationsParams": {
			func() any { return &ListApplicationsParams{} },
			`[25,3,true]`,
			`{"limit":25,"offset":3,"descending":true}`,
		},
		"GetApplicationParams": {
			func() any { return &GetApplicationParams{} },
			`["app"]`,
			`{"application":"app"}`,
		},
		"ListEpochsParams": {
			func() any { return &ListEpochsParams{} },
			`["app",["OPEN","CLOSED"],25,3,true,"0x2","0x9"]`,
			`{"application":"app","status":["OPEN","CLOSED"],"limit":25,"offset":3,"descending":true,"from":"0x2","to":"0x9"}`,
		},
		"GetEpochParams": {
			func() any { return &GetEpochParams{} },
			`["app","0x4"]`,
			`{"application":"app","epoch_index":"0x4"}`,
		},
		"GetEpochByVirtualIndexParams": {
			func() any { return &GetEpochByVirtualIndexParams{} },
			`["app","0x7"]`,
			`{"application":"app","virtual_index":"0x7"}`,
		},
		"GetLastAcceptedEpochIndexParams": {
			func() any { return &GetLastAcceptedEpochIndexParams{} },
			`["app"]`,
			`{"application":"app"}`,
		},
		"ListInputsParams": {
			func() any { return &ListInputsParams{} },
			`["app","0x4","sender","transaction-hash",25,3,true,"0x2","0x9"]`,
			`{"application":"app","epoch_index":"0x4","sender":"sender","transaction_hash":"transaction-hash",` +
				`"limit":25,"offset":3,"descending":true,"from":"0x2","to":"0x9"}`,
		},
		"GetInputParams": {
			func() any { return &GetInputParams{} },
			`["app","0x5"]`,
			`{"application":"app","input_index":"0x5"}`,
		},
		"GetProcessedInputCountParams": {
			func() any { return &GetProcessedInputCountParams{} },
			`["app"]`,
			`{"application":"app"}`,
		},
		"ListOutputsParams": {
			func() any { return &ListOutputsParams{} },
			`["app","0x4","0x5",["0x237a816f","0x10321e8b"],"voucher",25,3,true,"0x2","0x9",true]`,
			`{"application":"app","epoch_index":"0x4","input_index":"0x5",` +
				`"output_type":["0x237a816f","0x10321e8b"],"voucher_address":"voucher",` +
				`"limit":25,"offset":3,"descending":true,"from":"0x2","to":"0x9","executed":true}`,
		},
		"GetOutputParams": {
			func() any { return &GetOutputParams{} },
			`["app","0x6"]`,
			`{"application":"app","output_index":"0x6"}`,
		},
		"ListReportsParams": {
			func() any { return &ListReportsParams{} },
			`["app","0x4","0x5",25,3,true,"0x2","0x9"]`,
			`{"application":"app","epoch_index":"0x4","input_index":"0x5","limit":25,"offset":3,"descending":true,"from":"0x2","to":"0x9"}`,
		},
		"GetReportParams": {
			func() any { return &GetReportParams{} },
			`["app","0x7"]`,
			`{"application":"app","report_index":"0x7"}`,
		},
		"ListTournamentsParams": {
			func() any { return &ListTournamentsParams{} },
			`["app","0x4","0x2","parent-tournament","parent-match",25,3,true]`,
			`{"application":"app","epoch_index":"0x4","level":"0x2",` +
				`"parent_tournament_address":"parent-tournament","parent_match_id_hash":"parent-match",` +
				`"limit":25,"offset":3,"descending":true}`,
		},
		"GetTournamentParams": {
			func() any { return &GetTournamentParams{} },
			`["app","tournament"]`,
			`{"application":"app","address":"tournament"}`,
		},
		"ListCommitmentsParams": {
			func() any { return &ListCommitmentsParams{} },
			`["app","0x4","tournament",25,3,true]`,
			`{"application":"app","epoch_index":"0x4","tournament_address":"tournament","limit":25,"offset":3,"descending":true}`,
		},
		"GetCommitmentParams": {
			func() any { return &GetCommitmentParams{} },
			`["app","0x4","tournament","commitment"]`,
			`{"application":"app","epoch_index":"0x4","tournament_address":"tournament","commitment":"commitment"}`,
		},
		"ListMatchesParams": {
			func() any { return &ListMatchesParams{} },
			`["app","0x4","tournament",25,3,true]`,
			`{"application":"app","epoch_index":"0x4","tournament_address":"tournament","limit":25,"offset":3,"descending":true}`,
		},
		"GetMatchParams": {
			func() any { return &GetMatchParams{} },
			`["app","0x4","tournament","id-hash"]`,
			`{"application":"app","epoch_index":"0x4","tournament_address":"tournament","id_hash":"id-hash"}`,
		},
		"ListMatchAdvancesParams": {
			func() any { return &ListMatchAdvancesParams{} },
			`["app","0x4","tournament","id-hash",25,3,true]`,
			`{"application":"app","epoch_index":"0x4","tournament_address":"tournament",` +
				`"id_hash":"id-hash","limit":25,"offset":3,"descending":true}`,
		},
		"GetMatchAdvanceParams": {
			func() any { return &GetMatchAdvanceParams{} },
			`["app","0x4","tournament","id-hash","parent"]`,
			`{"application":"app","epoch_index":"0x4","tournament_address":"tournament","id_hash":"id-hash","parent":"parent"}`,
		},
		"ListWithdrawalsParams": {
			func() any { return &ListWithdrawalsParams{} },
			`["app","0x8",25,3,true]`,
			`{"application":"app","account_index":"0x8","limit":25,"offset":3,"descending":true}`,
		},
		"GetWithdrawalParams": {
			func() any { return &GetWithdrawalParams{} },
			`["app","0x8"]`,
			`{"application":"app","account_index":"0x8"}`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			expected := test.newTarget()
			require.NoError(t, json.Unmarshal([]byte(test.named), expected))

			actual := test.newTarget()
			require.NoError(t, UnmarshalParams(json.RawMessage(test.positional), actual))

			require.Equal(t, expected, actual)
		})
	}
}

func TestUnmarshalParamsEmptyRepresentationsLeaveTargetUnchanged(t *testing.T) {
	for name, data := range map[string]json.RawMessage{
		"omitted":     nil,
		"null":        json.RawMessage(`null`),
		"empty array": json.RawMessage(`[]`),
	} {
		t.Run(name, func(t *testing.T) {
			params := ListApplicationsParams{Limit: 7, Offset: 3, Descending: true}
			expected := params

			require.NoError(t, UnmarshalParams(data, &params))
			require.Equal(t, expected, params)
		})
	}
}

func TestUnmarshalParamsRejectsPositionalOverArity(t *testing.T) {
	var params GetApplicationParams
	err := UnmarshalParams(json.RawMessage(`["app","extra"]`), &params)

	require.EqualError(t, err, "error unmarshalling positional parameters, expected 1 params, got 2")
}

func TestUnmarshalParamsPositionalOrderSkipsIgnoredJSONFields(t *testing.T) {
	type paramsWithIgnoredField struct {
		First   string `json:"first"`
		Ignored string `json:"-"`
		Second  string `json:"second"`
	}

	params := paramsWithIgnoredField{Ignored: "unchanged"}
	require.NoError(t, UnmarshalParams(json.RawMessage(`["one","two"]`), &params))
	require.Equal(t, paramsWithIgnoredField{
		First:   "one",
		Ignored: "unchanged",
		Second:  "two",
	}, params)

	err := UnmarshalParams(json.RawMessage(`["one","two","extra"]`), &params)
	require.EqualError(t, err, "error unmarshalling positional parameters, expected 2 params, got 3")
}
