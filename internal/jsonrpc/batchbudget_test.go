// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMethodErrorListsExcludeBatchWideErrors(t *testing.T) {
	data, err := discoverSpec.ReadFile("jsonrpc-discover.json")
	require.NoError(t, err)
	var spec struct {
		Methods []struct {
			Name   string `json:"name"`
			Errors []struct {
				Ref string `json:"$ref"`
			} `json:"errors"`
		} `json:"methods"`
	}
	require.NoError(t, json.Unmarshal(data, &spec))

	for _, method := range spec.Methods {
		refs := make(map[string]bool, len(method.Errors))
		for _, methodErr := range method.Errors {
			name := methodErr.Ref[strings.LastIndex(methodErr.Ref, "/")+1:]
			refs[name] = true
		}
		require.False(t, refs["BatchListItemLimitExceeded"],
			"%s must not advertise a whole-batch error as a per-method error", method.Name)
		require.True(t, refs["TimeoutError"], "%s must advertise timeout responses", method.Name)
		require.True(t, refs["ResponseSizeLimitExceeded"],
			"%s must advertise response-size errors", method.Name)
	}
}

func TestBatchListItemLimitSupportsNamedAndPositionalParams(t *testing.T) {
	positionalAtLimit := map[string]string{
		"cartesi_listApplications":  `[10000]`,
		"cartesi_listEpochs":        `["app",null,10000]`,
		"cartesi_listInputs":        `["app",null,null,null,10000]`,
		"cartesi_listOutputs":       `["app",null,null,null,null,10000]`,
		"cartesi_listReports":       `["app",null,null,10000]`,
		"cartesi_listWithdrawals":   `["app",null,10000]`,
		"cartesi_listTournaments":   `["app",null,null,null,null,10000]`,
		"cartesi_listCommitments":   `["app",null,null,10000]`,
		"cartesi_listMatches":       `["app",null,null,10000]`,
		"cartesi_listMatchAdvances": `["app","0x0","tournament","id",10000]`,
	}

	for method, positional := range positionalAtLimit {
		t.Run(method, func(t *testing.T) {
			requests := []json.RawMessage{
				json.RawMessage(fmt.Sprintf(
					`{"jsonrpc":"2.0","method":%q,"params":%s,"id":1}`, method, positional)),
				json.RawMessage(
					`{"jsonrpc":"2.0","method":"cartesi_listApplications","params":{"limit":1},"id":2}`),
			}
			require.True(t, batchExceedsListItemLimit(requests))
		})
	}

	require.False(t, batchExceedsListItemLimit([]json.RawMessage{
		json.RawMessage(
			`{"jsonrpc":"2.0","method":"cartesi_listApplications","params":{"limit":6000},"id":1}`),
		json.RawMessage(
			`{"jsonrpc":"2.0","method":"cartesi_listOutputs","params":{"limit":4000},"id":2}`),
	}))
}

func TestBatchListItemLimitRegistryCoversEveryListHandler(t *testing.T) {
	for method := range jsonrpcHandlers {
		if strings.HasPrefix(method, "cartesi_list") {
			require.Contains(t, listParamsTypes, method)
		}
	}
}

func TestBatchListItemLimitNormalizesLimitsLikeHandlers(t *testing.T) {
	// A zero limit uses the default, while a value above the per-list maximum
	// is capped at that maximum.
	require.False(t, batchExceedsListItemLimit([]json.RawMessage{json.RawMessage(
		`{"jsonrpc":"2.0","method":"cartesi_listApplications","params":{"limit":0},"id":1}`,
	)}))
	require.False(t, batchExceedsListItemLimit([]json.RawMessage{json.RawMessage(
		`{"jsonrpc":"2.0","method":"cartesi_listApplications","params":{"limit":20000},"id":1}`,
	)}))
	require.True(t, batchExceedsListItemLimit([]json.RawMessage{
		json.RawMessage(
			`{"jsonrpc":"2.0","method":"cartesi_listApplications","params":{"limit":20000},"id":1}`),
		json.RawMessage(
			`{"jsonrpc":"2.0","method":"cartesi_listApplications","params":{"limit":1},"id":2}`),
	}))
}

func TestJSONRPCBatchRejectsListWorkOverLimitBeforeDispatch(t *testing.T) {
	s := newBatchTestService()
	var calls atomic.Int32
	withTestRPCHandler(t, s, "cartesi_listApplications", func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
		calls.Add(1)
		return true, nil
	})

	rr := serveRPC(t, s, []byte(`[
		{"jsonrpc":"2.0","method":"cartesi_listApplications","params":{"limit":6000},"id":1},
		{"jsonrpc":"2.0","method":"cartesi_listApplications","params":{"limit":4001},"id":2}
	]`))

	require.Equal(t, http.StatusOK, rr.Code)
	response := decodeRPCResponse(t, rr.Body.Bytes())
	requireRPCError(t, response, nil, JSONRPC_BATCH_LIST_ITEM_LIMIT_EXCEEDED)
	require.Equal(t, "Batch list item limit exceeded", response.Error.Message)
	require.Zero(t, calls.Load(), "an over-budget batch must be rejected before dispatch")
}

func TestJSONRPCBatchAllowsListWorkAtLimit(t *testing.T) {
	s := newBatchTestService()
	var calls atomic.Int32
	withTestRPCHandler(t, s, "cartesi_listApplications", func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
		calls.Add(1)
		return true, nil
	})

	rr := serveRPC(t, s, []byte(`[
		{"jsonrpc":"2.0","method":"cartesi_listApplications","params":{"limit":6000},"id":1},
		{"jsonrpc":"2.0","method":"cartesi_listApplications","params":{"limit":4000},"id":2}
	]`))

	require.Equal(t, http.StatusOK, rr.Code)
	require.Len(t, decodeRPCBatch(t, rr.Body.Bytes()), 2)
	require.Equal(t, int32(2), calls.Load())
}
