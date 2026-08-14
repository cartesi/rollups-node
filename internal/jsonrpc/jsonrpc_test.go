// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Tests for jsonrpc API using test tables of the database.
//
// for a simple coverage check, run:
// ```
// go test -C internal/jsonrpc/ -cover
// ```
//
// for an actual report, run:
// ```
// go test -C internal/jsonrpc/ -coverprofile=coverage.out
// go tool -C internal/jsonrpc/ cover -html=coverage.out
// ```
package jsonrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"time"

	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/jsonrpc/api"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type jsonrpcSchema struct {
	Methods []struct {
		Name string
	}
}

// failure: invalid JSON (extra ',' at the end)
func TestInvalidJSON(t *testing.T) {
	s := newTestService(t, t.Name())

	body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
		"jsonrpc": "2.0",
		"method": "",
		"params": {},
		"id": 0,
	}`))

	var resp RPCResponse
	assert.Nil(t, json.Unmarshal(body, &resp))
	assert.Equal(t, JSONRPC_PARSE_ERROR, resp.Error.Code)
	assert.Equal(t, "invalid request", resp.Error.Message)
}

// failure: invalid method
func TestInvalidMethod(t *testing.T) {
	s := newTestService(t, t.Name())

	body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
		"jsonrpc": "2.0",
		"method": "cartesi_invalidMethod",
		"params": {},
		"id": 0
	}`))

	resp := testRPCResponse[any]{}
	assert.Nil(t, json.Unmarshal(body, &resp))
	assert.Equal(t, JSONRPC_METHOD_NOT_FOUND, resp.Error.Code)
	assert.Equal(t, "Method not found", resp.Error.Message)
}

func TestJSONRPCSingleRequestReplacesResponseAtResponseBudget(t *testing.T) {
	s := newBatchTestService()
	const method = "test_large_single_result"
	largeResult := strings.Repeat("x", MAX_RESPONSE_SIZE)
	var called bool
	withTestRPCHandler(t, method, func(_ *Service, _ *http.Request, _ RPCRequest) (any, error) {
		called = true
		return largeResult, nil
	})

	body := []byte(fmt.Sprintf(
		`{"jsonrpc":"2.0","method":%q,"params":{"limit":10000},"id":1}`, method))
	require.Less(t, len(body), 1<<10, "the request cap must not be mistaken for the response cap")
	rr := serveRPC(t, s, body)

	require.True(t, called, "the request handler must run before its oversized response is replaced")
	require.Equal(t, http.StatusOK, rr.Code)
	response := decodeRPCResponse(t, rr.Body.Bytes())
	requireRPCError(t, response, float64(1), JSONRPC_RESPONSE_SIZE_LIMIT_EXCEEDED)
	require.Equal(t, "Response size limit exceeded", response.Error.Message)
}

// tests for jsonrpc methods grouped by method name.
// At the end we check if all methods ran at least once
func TestMethod(t *testing.T) {
	testHistogram := hist{}

	getName := func(name string) string {
		part, _ := strings.CutPrefix(name, "TestMethod/")
		return part
	}

	////////////////////////////////////////////////////////////////////////
	// rpc.discover
	////////////////////////////////////////////////////////////////////////
	t.Run("rpc.discover", func(t *testing.T) {
		method := getName(t.Name())

		// success: return discover file contents
		t.Run("success", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			contents, err := os.ReadFile("jsonrpc-discover.json")
			require.NoError(t, err)

			var expected any
			assert.Nil(t, json.Unmarshal(contents, &expected))

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "rpc.discover",
				"params": {},
				"id": 0
				}`))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getApplication
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getApplication", func(t *testing.T) {
		method := getName(t.Name())

		// failure: application not in the database -> resource not found
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(0)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getApplication",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: application is in the database -> retrieve application
		t.Run("present", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getApplication",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[model.Application]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, nr, nameToNumber(resp.Result.Data.Name))
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getNodeInfo
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getNodeInfo", func(t *testing.T) {
		method := getName(t.Name())

		// failure: evm reader not configured -> resource not found
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			body := s.doRequest(t, 0, []byte(`{
				"jsonrpc": "2.0",
				"method": "cartesi_getNodeInfo",
				"params": {},
				"id": 0
			}`))

			resp := testRPCResponse[any]{}
			require.NoError(t, json.Unmarshal(body, &resp))
			require.NotNil(t, resp.Error)
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "EVM Reader config not found", resp.Error.Message)
		})

		// success: combine persisted node configuration with the build version
		t.Run("present", func(t *testing.T) {
			testHistogram.inc(method)
			ctx := context.Background()
			s := newTestService(t, t.Name())

			chainID := uint64(0xdeadbeef)
			defaultBlock := model.DefaultBlock_Safe
			err := repository.SaveNodeConfig(ctx, s.repository,
				&model.NodeConfig[evmreader.PersistentConfig]{
					Key: evmreader.EvmReaderConfigKey,
					Value: evmreader.PersistentConfig{
						ChainID:      chainID,
						DefaultBlock: defaultBlock,
					},
				},
			)
			require.NoError(t, err)

			body := s.doRequest(t, 0, []byte(`{
				"jsonrpc": "2.0",
				"method": "cartesi_getNodeInfo",
				"params": {},
				"id": 0
			}`))

			resp := testRPCResponse[api.NodeInfo]{}
			require.NoError(t, json.Unmarshal(body, &resp))
			require.Nil(t, resp.Error)
			assert.Equal(t, hexutil.EncodeUint64(chainID), resp.Result.Data.ChainID)
			assert.Equal(t, version.BuildVersion, resp.Result.Data.Version)
			assert.Equal(t, string(defaultBlock), resp.Result.Data.DefaultBlock)
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getChainId
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getChainId", func(t *testing.T) {
		method := getName(t.Name())

		// failure: evm reader not configured -> resource not found
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, "getChainIdFailure")

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getChainId",
				"params": {},
				"id": 0
				}`))
			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "EVM Reader config not found", resp.Error.Message)
		})

		// success: evm reader configured -> retrieve chain id
		t.Run("present", func(t *testing.T) {
			testHistogram.inc(method)
			ctx := context.Background()
			s := newTestService(t, "getChainIdFailure")

			// NodeConfig provision
			nr := uint64(0xdeadbeef)
			err := repository.SaveNodeConfig(ctx, s.repository,
				&model.NodeConfig[evmreader.PersistentConfig]{
					Key: evmreader.EvmReaderConfigKey,
					Value: evmreader.PersistentConfig{
						ChainID: nr,
					},
				},
			)
			require.NoError(t, err, "on test case: %v, when saving evm reader config", t.Name())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getChainId",
				"params": {},
				"id": 0
				}`))
			resp := testRPCResponse[hex64]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, nr, uint64(resp.Result.Data))
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getEpoch
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getEpoch", func(t *testing.T) {
		method := getName(t.Name())

		// failure: epoch_index not hex encoded -> invalid param
		t.Run("malformedEpochIndex", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			app := uint64(1)
			nr := uint64(0)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getEpoch",
				"params": {
				"application": "%v",
				"epoch_index": "%v"
				},
				"id": 0
				}`, numberToName(app), nr))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Equal(t, "Invalid epoch index: expected hex encoded value", resp.Error.Message)
		})

		// failure: epoch not in the database -> resource not found
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			nr := uint64(0)

			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(nr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getEpoch",
				"params": {
				"application": "%v",
				"epoch_index": "%v"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(nr+1)))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Epoch not found", resp.Error.Message)
		})

		// failure: application not in the database -> application not found
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(0xdeadbeef)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getEpoch",
				"params": {
				"application": "%v",
				"epoch_index": "%v"
				},
				"id": 0
				}`, numberToName(nr), hexutil.EncodeUint64(0)))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: epoch is in the database -> retrieve epoch
		t.Run("present", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			nr := uint64(1)

			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(nr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getEpoch",
				"params": {
				"application": "%v",
				"epoch_index": "%v"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(nr)))

			resp := testRPCResponse[*model.Epoch]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, nr, resp.Result.Data.Index)
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getEpochByVirtualIndex
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getEpochByVirtualIndex", func(t *testing.T) {
		method := getName(t.Name())

		// failure: virtual_index not hex encoded -> invalid param
		t.Run("malformedVirtualIndex", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getEpochByVirtualIndex",
				"params": {
					"application": "%v",
					"virtual_index": 0
				},
				"id": 0
			}`, numberToName(1)))

			resp := testRPCResponse[any]{}
			require.NoError(t, json.Unmarshal(body, &resp))
			require.NotNil(t, resp.Error)
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Equal(t, "Invalid parameters", resp.Error.Message)
		})

		// failure: virtual index not in the database -> resource not found
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(5).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getEpochByVirtualIndex",
				"params": {
					"application": "%v",
					"virtual_index": "%v"
				},
				"id": 0
			}`, numberToName(app), hexutil.EncodeUint64(1)))

			resp := testRPCResponse[any]{}
			require.NoError(t, json.Unmarshal(body, &resp))
			require.NotNil(t, resp.Error)
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Epoch not found", resp.Error.Message)
		})

		// failure: application not in the database -> application not found
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getEpochByVirtualIndex",
				"params": {
					"application": "%v",
					"virtual_index": "0x0"
				},
				"id": 0
			}`, numberToName(0xdeadbeef)))

			resp := testRPCResponse[any]{}
			require.NoError(t, json.Unmarshal(body, &resp))
			require.NotNil(t, resp.Error)
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: lookup uses the dense virtual index, not the physical epoch index
		t.Run("presentWithDivergentPhysicalIndex", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(5).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getEpochByVirtualIndex",
				"params": {
					"application": "%v",
					"virtual_index": "0x0"
				},
				"id": 0
			}`, numberToName(app)))

			resp := testRPCResponse[*model.Epoch]{}
			require.NoError(t, json.Unmarshal(body, &resp))
			require.Nil(t, resp.Error)
			require.NotNil(t, resp.Result.Data)
			assert.Equal(t, uint64(5), resp.Result.Data.Index)
			assert.Equal(t, uint64(0), resp.Result.Data.VirtualIndex)
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getInput
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getInput", func(t *testing.T) {
		method := getName(t.Name())

		// failure: input_index not hex encoded -> invalid param
		t.Run("malformedInputIndex", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			app := uint64(1)
			nr := uint64(0)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getInput",
				"params": {
				"application": "%v",
				"input_index": "%v"
				},
				"id": 0
				}`, numberToName(app), nr))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Equal(t, "Invalid input index: expected hex encoded value", resp.Error.Message)
		})

		// failure: input_index not in database -> absent input
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(2)
			enr := uint64(1)
			inr := uint64(0)
			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(enr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getInput",
				"params": {
				"application": "%v",
				"input_index": "%v"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(inr)))

			resp := testRPCResponse[*model.Input]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Input not found", resp.Error.Message)
		})

		// failure: application not in the database -> application not found
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(0xdeadbeef)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getInput",
				"params": {
				"application": "%v",
				"input_index": "%v"
				},
				"id": 0
				}`, numberToName(nr), hexutil.EncodeUint64(0)))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: input_index of EvmAdvance in the database -> retrieve input
		t.Run("success", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(2)
			enr := uint64(1)
			inr := uint64(0)
			appID := s.newTestApplication(ctx, t, app)
			epoch := repotest.NewEpochBuilder(appID).
				WithIndex(enr).
				WithStatus(model.EpochStatus_ClaimAccepted).
				Build()
			input := repotest.NewInputBuilder().
				WithIndex(inr).
				WithRawData(emptyInput()).
				Build()
			s.createTestEpochWithInput(ctx, t, numberToName(app), epoch, input)
			s.advanceInput(ctx, t, appID, enr, inr, nil, nil)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getInput",
				"params": {
				"application": "%v",
				"input_index": "%v"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(inr)))

			resp := testRPCResponse[*model.Input]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, enr, resp.Result.Data.EpochIndex)
			assert.Equal(t, inr, resp.Result.Data.Index)
		})

		// TODO: also test DecodedInput
	})

	////////////////////////////////////////////////////////////////////////
	// getLastAcceptedEpochIndex
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getLastAcceptedEpochIndex", func(t *testing.T) {
		method := getName(t.Name())

		// failure: application not in the database -> application not found
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(0xdeadbeef)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getLastAcceptedEpochIndex",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[hex64]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// failure: application exists but has no accepted epoch yet ->
		// epoch not found (the pollable "not yet" signal)
		t.Run("no_accepted_epoch", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(0)
			s.newTestApplication(ctx, t, nr)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getLastAcceptedEpochIndex",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[hex64]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Epoch not found", resp.Error.Message)
		})

		// success: epoch is in the database -> retrieve epoch index
		t.Run("success", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(0)
			epochIndex := uint64(0xdeadbeef)

			appID := s.newTestApplication(ctx, t, nr)
			s.createTestEpoch(ctx, t, numberToName(nr),
				repotest.NewEpochBuilder(appID).
					WithIndex(epochIndex).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getLastAcceptedEpochIndex",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[hex64]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, epochIndex, uint64(resp.Result.Data))
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getNodeVersion
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getNodeVersion", func(t *testing.T) {
		method := getName(t.Name())

		// success: -> version.BuildVersion
		t.Run("success", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getNodeVersion",
				"params": {},
				"id": 0
				}`))

			resp := testRPCResponse[string]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, version.BuildVersion, resp.Result.Data)
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getOutput
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getOutput", func(t *testing.T) {
		method := getName(t.Name())

		// failure: output_index not hex encoded -> invalid param
		t.Run("malformed", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			app := uint64(1)
			nr := uint64(0)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getOutput",
				"params": {
				"application": "%v",
				"output_index": "%v"
				},
				"id": 0
				}`, numberToName(app), nr))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Equal(t, "Invalid output index: expected hex encoded value", resp.Error.Message)
		})

		// failure: input_index not in database -> absent input
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(2)
			enr := uint64(1)
			inr := uint64(0)
			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(enr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getOutput",
				"params": {
				"application": "%v",
				"output_index": "%v"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(inr)))

			resp := testRPCResponse[*model.Output]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Output not found", resp.Error.Message)
		})

		// failure: application not in the database -> application not found
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(0xdeadbeef)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getOutput",
				"params": {
				"application": "%v",
				"output_index": "%v"
				},
				"id": 0
				}`, numberToName(nr), hexutil.EncodeUint64(0)))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: output_index of Voucher in the database -> retrieve output
		t.Run("success", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(3)
			enr := uint64(1)
			inr := uint64(1)
			onr := uint64(0)
			appID := s.newTestApplication(ctx, t, app)
			epoch := repotest.NewEpochBuilder(appID).
				WithIndex(enr).
				WithStatus(model.EpochStatus_ClaimAccepted).
				Build()
			input := repotest.NewInputBuilder().
				WithIndex(inr).
				WithRawData(emptyInput()).
				Build()
			s.createTestEpochWithInput(ctx, t, numberToName(app), epoch, input)
			s.advanceInput(ctx, t, appID, enr, inr, [][]byte{emptyVoucher()}, nil)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getOutput",
				"params": {
				"application": "%v",
				"output_index": "%v"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(onr)))

			type Result struct {
				EpochIndex hex64  `json:"epoch_index"`
				InputIndex hex64  `json:"input_index"`
				Index      hex64  `json:"index"`
				RawData    string `json:"raw_data"` // hex encoded

				DecodedData *api.DecodedData `json:"decoded_data,omitempty"`

				// ... (ignore the rest of fields for test
			}

			resp := testRPCResponse[Result]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, inr, uint64(resp.Result.Data.InputIndex))
			assert.Equal(t, onr, uint64(resp.Result.Data.Index))
			assert.Equal(t, "0xdeadbeef", resp.Result.Data.DecodedData.Value)
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getProcessedInputCount
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getProcessedInputCount", func(t *testing.T) {
		method := getName(t.Name())

		// failure: application not in the database -> application not found
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			app := uint64(1)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getProcessedInputCount",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(app)))

			resp := testRPCResponse[hex64]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: application has no inputs -> 0
		t.Run("absentInput", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)

			s.newTestApplication(ctx, t, app)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getProcessedInputCount",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(app)))

			resp := testRPCResponse[hex64]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, uint64(0), uint64(resp.Result.Data))
		})

		t.Run("processedInputs", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			appID := s.newTestApplication(ctx, t, app)
			epoch := repotest.NewEpochBuilder(appID).
				WithIndex(0).
				WithStatus(model.EpochStatus_ClaimAccepted).
				Build()
			inputs := []*model.Input{
				repotest.NewInputBuilder().WithIndex(0).WithRawData(emptyInput()).Build(),
				repotest.NewInputBuilder().WithIndex(1).WithRawData(emptyInput()).Build(),
			}
			err := s.repository.CreateEpochsAndInputs(
				ctx,
				numberToName(app),
				map[*model.Epoch][]*model.Input{epoch: inputs},
				10,
			)
			require.NoError(t, err)
			s.advanceInput(ctx, t, appID, 0, 0, nil, nil)
			s.advanceInput(ctx, t, appID, 0, 1, nil, nil)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getProcessedInputCount",
				"params": { "application": "%s" },
				"id": 0
			}`, numberToName(app)))

			resp := testRPCResponse[hex64]{}
			require.NoError(t, json.Unmarshal(body, &resp))
			assert.Nil(t, resp.Error)
			assert.Equal(t, uint64(2), uint64(resp.Result.Data))
		})
	})

	for _, methodName := range []string{
		"cartesi_getExecutedOutputCount",
		"cartesi_getPendingExecutableOutputCount",
	} {
		t.Run(methodName, func(t *testing.T) {
			method := getName(t.Name())

			t.Run("absentApplication", func(t *testing.T) {
				testHistogram.inc(method)
				s := newTestService(t, t.Name())

				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "%s",
					"params": { "application": "%s" },
					"id": 0
				}`, method, numberToName(1)))

				resp := testRPCResponse[hex64]{}
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
				assert.Equal(t, "Application not found", resp.Error.Message)
			})

			t.Run("existingApplicationWithNoOutputs", func(t *testing.T) {
				testHistogram.inc(method)
				s := newTestService(t, t.Name())
				app := uint64(1)
				s.newTestApplication(context.Background(), t, app)

				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "%s",
					"params": { "application": "%s" },
					"id": 0
				}`, method, numberToName(app)))

				resp := testRPCResponse[hex64]{}
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Nil(t, resp.Error)
				assert.Equal(t, uint64(0), uint64(resp.Result.Data))
			})

			t.Run("outputsPresent", func(t *testing.T) {
				testHistogram.inc(method)
				s := newTestService(t, t.Name())
				ctx := context.Background()

				app := uint64(1)
				appID := s.newTestApplication(ctx, t, app)
				epoch := repotest.NewEpochBuilder(appID).
					WithIndex(0).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build()
				input := repotest.NewInputBuilder().
					WithIndex(0).
					WithRawData(emptyInput()).
					Build()
				s.createTestEpochWithInput(ctx, t, numberToName(app), epoch, input)
				s.advanceInput(ctx, t, appID, 0, 0, [][]byte{
					emptyVoucher(),
					{0x10, 0x32, 0x1e, 0x8b},
					{0xc2, 0x58, 0xd6, 0xe5},
				}, nil)

				txHash := common.HexToHash("0x1")
				err := s.repository.UpdateOutputsExecution(
					ctx,
					numberToName(app),
					[]*model.Output{
						{InputEpochApplicationID: appID, Index: 0, ExecutionTransactionHash: &txHash},
						{InputEpochApplicationID: appID, Index: 1, ExecutionTransactionHash: &txHash},
					},
					10,
				)
				require.NoError(t, err)

				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "%s",
					"params": { "application": "%s" },
					"id": 0
				}`, method, numberToName(app)))

				resp := testRPCResponse[hex64]{}
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Nil(t, resp.Error)
				expected := uint64(2)
				if method == "cartesi_getPendingExecutableOutputCount" {
					expected = 0
				}
				assert.Equal(t, expected, uint64(resp.Result.Data))
			})
		})
	}

	////////////////////////////////////////////////////////////////////////
	// getReport
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getReport", func(t *testing.T) {
		method := getName(t.Name())

		// failure: report_index not hex encoded -> invalid param
		t.Run("malformed", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			app := uint64(1)
			reportIndex := uint64(0)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getReport",
				"params": {
				"application": "%v",
				"report_index": "%v"
				},
				"id": 0
				}`, numberToName(app), reportIndex))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Equal(t, "Invalid report index: expected hex encoded value", resp.Error.Message)
		})

		// failure: report_index not in database -> absent report
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(2)
			enr := uint64(1)
			rnr := uint64(0)
			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(enr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getReport",
				"params": {
				"application": "%v",
				"report_index": "%v"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(rnr)))

			resp := testRPCResponse[*model.Report]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Report not found", resp.Error.Message)
		})

		// failure: application not in the database -> application not found
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(0xdeadbeef)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getReport",
				"params": {
				"application": "%v",
				"report_index": "%v"
				},
				"id": 0
				}`, numberToName(nr), hexutil.EncodeUint64(0)))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: output_index of Voucher in the database -> retrieve output
		t.Run("success", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(3)
			enr := uint64(1)
			inr := uint64(1)
			onr := uint64(0)
			appID := s.newTestApplication(ctx, t, app)
			epoch := repotest.NewEpochBuilder(appID).
				WithIndex(enr).
				WithStatus(model.EpochStatus_ClaimAccepted).
				Build()
			input := repotest.NewInputBuilder().
				WithIndex(inr).
				WithRawData(emptyInput()).
				Build()
			s.createTestEpochWithInput(ctx, t, numberToName(app), epoch, input)
			s.advanceInput(ctx, t, appID, enr, inr, nil, [][]byte{emptyVoucher()})

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getReport",
				"params": {
				"application": "%v",
				"report_index": "%v"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(onr)))

			type Result struct {
				EpochIndex hex64  `json:"epoch_index"`
				InputIndex hex64  `json:"input_index"`
				Index      hex64  `json:"index"`
				RawData    string `json:"raw_data"` // hex encoded

				// ... (ignore the rest of fields for test
			}

			resp := testRPCResponse[Result]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, inr, uint64(resp.Result.Data.InputIndex))
			assert.Equal(t, onr, uint64(resp.Result.Data.Index))
		})
	})

	////////////////////////////////////////////////////////////////////////
	// listApplications
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_listApplications", func(t *testing.T) {
		method := getName(t.Name())

		// success: no application is in the database -> 0
		t.Run("empty", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listApplications",
				"params": {
				"limit": %v,
				"offset": %v,
				"descending": %v
				},
				"id": 0
				}`, 0, 0, false))

			resp := testRPCResponse[[]model.Application]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, 0, len(resp.Result.Data))
		})

		// success: 1 application is in the database -> 1
		t.Run("one", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listApplications",
				"params": {
				"limit": %v,
				"offset": %v,
				"descending": %v
				},
				"id": 0
				}`, 1, 0, false))

			resp := testRPCResponse[[]model.Application]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, 1, len(resp.Result.Data))
			assert.Equal(t, numberToName(nr), resp.Result.Data[0].Name)
		})

		// success: 1 application is in the database (array params) -> 1
		t.Run("oneArrayParams", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listApplications",
				"params": [%v, %v, %v],
				"id": 0
				}`, 1, 0, false))

			resp := testRPCResponse[[]model.Application]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, 1, len(resp.Result.Data))
			assert.Equal(t, numberToName(nr), resp.Result.Data[0].Name)
		})

		// success: 1 application is in the database (array params) -> 1
		t.Run("emptyArrayParams", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, []byte(`{
				"jsonrpc": "2.0",
				"method": "cartesi_listApplications",
				"id": 0
			}`))
			resp := testRPCResponse[[]model.Application]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, 1, len(resp.Result.Data))
			assert.Equal(t, numberToName(nr), resp.Result.Data[0].Name)
		})

		// success: many applications is in the database -> limit (many - 1)
		t.Run("many", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			many := uint64(100)
			limit := uint64(many / 2)
			for i := range many {
				s.newTestApplication(ctx, t, i)
			}

			{ // offset == 0, descending = false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listApplications",
					"params": {
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, limit, 0, false))

				resp := testRPCResponse[[]model.Application]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i
					assert.Equal(t, nr, nameToNumber(resp.Result.Data[i].Name))
				}
			}

			{ // offset == 1, descending == false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listApplications",
					"params": {
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, limit, 1, false))

				resp := testRPCResponse[[]model.Application]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i + 1
					assert.Equal(t, nr, nameToNumber(resp.Result.Data[i].Name))
				}
			}

			{ // offset == 0, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listApplications",
					"params": {
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, limit, 0, true))

				resp := testRPCResponse[[]model.Application]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 1
					assert.Equal(t, nr, nameToNumber(resp.Result.Data[i].Name))
				}
			}

			{ // offset == 1, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listApplications",
					"params": {
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, limit, 1, true))

				resp := testRPCResponse[[]model.Application]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 2
					assert.Equal(t, nr, nameToNumber(resp.Result.Data[i].Name))
				}
			}
		})
	})

	////////////////////////////////////////////////////////////////////////////////
	// listEpochs
	////////////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_listEpochs", func(t *testing.T) {
		method := getName(t.Name())

		// failure: no epoch in the database -> no application
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(1)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listEpochs",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Epoch]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: no epoch in the database -> 0
		t.Run("absentEpoch", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listEpochs",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Epoch]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, 0, len(resp.Result.Data))
		})

		// failure: query invalid status -> invalid params
		t.Run("invalid", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listEpochs",
				"params": {
				"application": "%v",
				"status": "INVALID"
				},
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Epoch]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Equal(t, "Invalid epoch status: invalid value 'INVALID' for EpochStatus enum", resp.Error.Message)
		})

		// failure: any invalid status in a list -> invalid params
		t.Run("invalidInList", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			body := s.doRequest(t, 0, []byte(`{
				"jsonrpc": "2.0",
				"method": "cartesi_listEpochs",
				"params": {
					"application": "app",
					"status": ["OPEN", "INVALID"]
				},
				"id": 0
			}`))

			resp := testRPCResponse[[]model.Epoch]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Equal(t, "Invalid epoch status: invalid value 'INVALID' for EpochStatus enum", resp.Error.Message)
		})

		// failure: an explicitly empty status list -> invalid params
		t.Run("emptyStatusList", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			body := s.doRequest(t, 0, []byte(`{
				"jsonrpc": "2.0",
				"method": "cartesi_listEpochs",
				"params": {
					"application": "app",
					"status": []
				},
				"id": 0
			}`))

			resp := testRPCResponse[[]model.Epoch]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Equal(t, "Invalid epoch status: expected at least one status", resp.Error.Message)
		})

		// success: status may contain multiple values
		t.Run("multipleStatuses", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			appID := s.newTestApplication(ctx, t, nr)
			for i, status := range []model.EpochStatus{
				model.EpochStatus_Open,
				model.EpochStatus_Closed,
				model.EpochStatus_ClaimAccepted,
			} {
				s.createTestEpoch(ctx, t, numberToName(nr),
					repotest.NewEpochBuilder(appID).
						WithIndex(uint64(i)).
						WithStatus(status).
						Build())
			}

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listEpochs",
				"params": {
					"application": "%v",
					"status": ["OPEN", "CLOSED"]
				},
				"id": 0
			}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Epoch]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Nil(t, resp.Error)
			assert.Len(t, resp.Result.Data, 2)
			assert.Equal(t, model.EpochStatus_Open, resp.Result.Data[0].Status)
			assert.Equal(t, model.EpochStatus_Closed, resp.Result.Data[1].Status)
		})

		// success: many epochs is in the database -> limit
		t.Run("many", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			many := uint64(100)
			limit := uint64(many / 2)
			appID := s.newTestApplication(ctx, t, nr)
			for i := range many {
				s.createTestEpoch(ctx, t, numberToName(nr),
					repotest.NewEpochBuilder(appID).
						WithIndex(i).
						WithStatus(model.EpochStatus_ClaimAccepted).
						Build())
			}

			{ // offset == 0, descending = false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listEpochs",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(nr), limit, 0, false))

				resp := testRPCResponse[[]model.Epoch]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i
					assert.Equal(t, nr, resp.Result.Data[i].Index)
				}
			}

			{ // offset == 0, descending = false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listEpochs",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(nr), limit, 1, false))

				resp := testRPCResponse[[]model.Epoch]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i + 1
					assert.Equal(t, nr, resp.Result.Data[i].Index)
				}
			}

			{ // offset == 0, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listEpochs",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(nr), limit, 0, true))

				resp := testRPCResponse[[]model.Epoch]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 1
					assert.Equal(t, nr, resp.Result.Data[i].Index)
				}
			}

			{ // offset == 1, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listEpochs",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(nr), limit, 1, true))

				resp := testRPCResponse[[]model.Epoch]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 2
					assert.Equal(t, nr, resp.Result.Data[i].Index)
				}
			}

			{ // inclusive index range
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listEpochs",
					"params": {"application": "%v", "from": "0x2", "to": "0x4"},
					"id": 0
				}`, numberToName(nr)))

				resp := testRPCResponse[[]model.Epoch]{}
				require.NoError(t, json.Unmarshal(body, &resp))
				require.Len(t, resp.Result.Data, 3)
				assert.Equal(t, []uint64{2, 3, 4}, []uint64{
					resp.Result.Data[0].Index, resp.Result.Data[1].Index, resp.Result.Data[2].Index,
				})
			}
		})
	})

	////////////////////////////////////////////////////////////////////////
	// listInputs
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_listInputs", func(t *testing.T) {
		method := getName(t.Name())

		// failure: no epoch in the database -> no application
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(1)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listInputs",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Epoch]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: no epoch in the database -> 0
		t.Run("absentInput", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listInputs",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Epoch]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, 0, len(resp.Result.Data))
		})

		t.Run("filterByTransactionHash", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			appID := s.newTestApplication(ctx, t, app)
			epoch := repotest.NewEpochBuilder(appID).
				WithIndex(0).
				WithStatus(model.EpochStatus_Closed).
				WithBlocks(0, 29).
				WithInputBounds(0, 2).
				Build()
			txHash := common.HexToHash("0xcafe")
			input0 := repotest.NewInputBuilder().
				WithIndex(0).
				WithRawData(emptyInput()).
				WithTransactionHash(txHash).
				WithLogIndex(10).
				Build()
			input1 := repotest.NewInputBuilder().
				WithIndex(1).
				WithRawData(emptyInput()).
				WithTransactionHash(repotest.UniqueHash()).
				WithLogIndex(11).
				Build()
			input2 := repotest.NewInputBuilder().
				WithIndex(2).
				WithRawData(emptyInput()).
				WithTransactionHash(txHash).
				WithLogIndex(12).
				Build()
			err := s.repository.CreateEpochsAndInputs(ctx, numberToName(app),
				map[*model.Epoch][]*model.Input{epoch: {input0, input1, input2}}, 30)
			require.NoError(t, err)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listInputs",
				"params": {
				"application": "%v",
				"transaction_hash": "%v"
				},
				"id": 0
				}`, numberToName(app), txHash.Hex()))

			resp := testRPCResponse[[]map[string]json.RawMessage]{}
			require.NoError(t, json.Unmarshal(body, &resp))
			require.Nil(t, resp.Error)
			require.Len(t, resp.Result.Data, 2)

			var index0, index1 hex64
			require.NoError(t, json.Unmarshal(resp.Result.Data[0]["index"], &index0))
			require.NoError(t, json.Unmarshal(resp.Result.Data[1]["index"], &index1))
			assert.Equal(t, uint64(0), uint64(index0))
			assert.Equal(t, uint64(2), uint64(index1))

			// log_index must carry the seeded values, hex-encoded like the
			// other uint64 fields.
			assert.JSONEq(t, `"0xa"`, string(resp.Result.Data[0]["log_index"]))
			assert.JSONEq(t, `"0xc"`, string(resp.Result.Data[1]["log_index"]))

			for _, input := range resp.Result.Data {
				assert.JSONEq(t, fmt.Sprintf("%q", txHash.Hex()), string(input["transaction_hash"]))
				assert.NotContains(t, input, "transaction_reference")
			}

			body = s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listInputs",
				"params": {"application": "%v", "from": "0x1", "to": "0x2"},
				"id": 0
			}`, numberToName(app)))
			resp = testRPCResponse[[]map[string]json.RawMessage]{}
			require.NoError(t, json.Unmarshal(body, &resp))
			require.Len(t, resp.Result.Data, 2)
			assert.JSONEq(t, `"0x1"`, string(resp.Result.Data[0]["index"]))
			assert.JSONEq(t, `"0x2"`, string(resp.Result.Data[1]["index"]))
		})

		// failure: malformed transaction hash -> invalid params, not an
		// empty result from a silently padded/truncated hash.
		t.Run("invalidTransactionHash", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			for _, malformed := range []string{"0xcafe", "not-a-hash", "0x" + strings.Repeat("ab", 33)} {
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listInputs",
					"params": {
					"application": "%v",
					"transaction_hash": "%v"
					},
					"id": 0
					}`, numberToName(nr), malformed))

				resp := testRPCResponse[[]model.Input]{}
				require.NoError(t, json.Unmarshal(body, &resp))
				require.NotNil(t, resp.Error, "expected error for %q", malformed)
				assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			}
		})
	})

	////////////////////////////////////////////////////////////////////////
	// listOutputs
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_listOutputs", func(t *testing.T) {
		method := getName(t.Name())

		// failure: no epoch in the database -> no application
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(1)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listOutputs",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Epoch]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: no epoch in the database -> 0
		t.Run("absentOutput", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listOutputs",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Epoch]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, 0, len(resp.Result.Data))
		})

		// success: many outputs in the database ordered correctly
		t.Run("success", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(3)
			enr := uint64(1)
			inr := uint64(1)
			appID := s.newTestApplication(ctx, t, app)
			epoch := repotest.NewEpochBuilder(appID).
				WithIndex(enr).
				WithStatus(model.EpochStatus_ClaimAccepted).
				Build()
			input := repotest.NewInputBuilder().
				WithIndex(inr).
				WithRawData(emptyInput()).
				Build()
			s.createTestEpochWithInput(ctx, t, numberToName(app), epoch, input)

			many := uint64(100)
			limit := uint64(many / 2)
			outputData := make([][]byte, many)
			for i := range many {
				outputData[i] = emptyVoucher()
			}
			s.advanceInput(ctx, t, appID, enr, inr, outputData, nil)

			type Result struct {
				EpochIndex hex64  `json:"epoch_index"`
				InputIndex hex64  `json:"input_index"`
				Index      hex64  `json:"index"`
				RawData    string `json:"raw_data"` // hex encoded

				DecodedData *api.DecodedData `json:"decoded_data,omitempty"`

				// ... (ignore the rest of fields for test
			}

			{ // offset == 0, descending = false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listOutputs",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 0, false))

				resp := testRPCResponse[[]Result]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i
					assert.Equal(t, nr, uint64(resp.Result.Data[i].Index))
				}
			}

			{ // offset == 1, descending == false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listOutputs",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 1, false))

				resp := testRPCResponse[[]Result]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i + 1
					assert.Equal(t, nr, uint64(resp.Result.Data[i].Index))
				}
			}

			{ // offset == 0, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listOutputs",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 0, true))

				resp := testRPCResponse[[]Result]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 1
					assert.Equal(t, nr, uint64(resp.Result.Data[i].Index))
				}
			}

			{ // offset == 1, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listOutputs",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 1, true))

				resp := testRPCResponse[[]Result]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 2
					assert.Equal(t, nr, uint64(resp.Result.Data[i].Index))
				}
			}

			{ // inclusive index range
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listOutputs",
					"params": {"application": "%v", "from": "0x2", "to": "0x4"},
					"id": 0
				}`, numberToName(app)))

				resp := testRPCResponse[[]Result]{}
				require.NoError(t, json.Unmarshal(body, &resp))
				require.Len(t, resp.Result.Data, 3)
				for i, expected := range []uint64{2, 3, 4} {
					assert.Equal(t, expected, uint64(resp.Result.Data[i].Index))
				}
			}
		})

		t.Run("executedWithOutputTypeList", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()
			app := uint64(4)
			appID := s.newTestApplication(ctx, t, app)
			epoch := repotest.NewEpochBuilder(appID).WithStatus(model.EpochStatus_ClaimAccepted).Build()
			input := repotest.NewInputBuilder().WithRawData(emptyInput()).Build()
			s.createTestEpochWithInput(ctx, t, numberToName(app), epoch, input)
			s.advanceInput(ctx, t, appID, 0, 0, [][]byte{
				emptyVoucher(),
				{0x10, 0x32, 0x1e, 0x8b},
				{0xc2, 0x58, 0xd6, 0xe5},
				emptyVoucher(),
			}, nil)

			txHash := common.HexToHash("0x1")
			err := s.repository.UpdateOutputsExecution(ctx, numberToName(app), []*model.Output{{
				InputEpochApplicationID: appID, Index: 3, ExecutionTransactionHash: &txHash,
			}}, 10)
			require.NoError(t, err)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listOutputs",
				"params": {
					"application": "%v",
					"executed": true,
					"output_type": ["0x237a816f", "0x10321e8b"]
				},
				"id": 0
			}`, numberToName(app)))

			resp := testRPCResponse[[]model.Output]{}
			require.NoError(t, json.Unmarshal(body, &resp))
			require.Nil(t, resp.Error)
			require.Len(t, resp.Result.Data, 1)
			assert.Equal(t, uint64(3), resp.Result.Data[0].Index)
		})
	})

	////////////////////////////////////////////////////////////////////////
	// listReports
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_listReports", func(t *testing.T) {
		method := getName(t.Name())

		// failure: no epoch in the database -> no application
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(1)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listReports",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Epoch]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: no epoch in the database -> 0
		t.Run("empty", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listReports",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Epoch]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, 0, len(resp.Result.Data))
		})

		// success: many reports
		t.Run("many", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(3)
			enr := uint64(1)
			inr := uint64(1)
			appID := s.newTestApplication(ctx, t, app)
			epoch := repotest.NewEpochBuilder(appID).
				WithIndex(enr).
				WithStatus(model.EpochStatus_ClaimAccepted).
				Build()
			input := repotest.NewInputBuilder().
				WithIndex(inr).
				WithRawData(emptyInput()).
				Build()
			s.createTestEpochWithInput(ctx, t, numberToName(app), epoch, input)

			many := uint64(100)
			limit := uint64(many / 2)
			reportData := make([][]byte, many)
			for i := range many {
				reportData[i] = emptyVoucher()
			}
			s.advanceInput(ctx, t, appID, enr, inr, nil, reportData)

			type Result struct {
				EpochIndex hex64  `json:"epoch_index"`
				InputIndex hex64  `json:"input_index"`
				Index      hex64  `json:"index"`
				RawData    string `json:"raw_data"` // hex encoded

				// ... (ignore the rest of fields for test
			}

			{ // offset == 0, descending = false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listReports",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 0, false))

				resp := testRPCResponse[[]Result]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i
					assert.Equal(t, nr, uint64(resp.Result.Data[i].Index))
				}
			}

			{ // offset == 1, descending == false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listReports",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 1, false))

				resp := testRPCResponse[[]Result]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i + 1
					assert.Equal(t, nr, uint64(resp.Result.Data[i].Index))
				}
			}

			{ // offset == 0, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listReports",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 0, true))

				resp := testRPCResponse[[]Result]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 1
					assert.Equal(t, nr, uint64(resp.Result.Data[i].Index))
				}
			}

			{ // offset == 1, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listReports",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 1, true))

				resp := testRPCResponse[[]Result]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 2
					assert.Equal(t, nr, uint64(resp.Result.Data[i].Index))
				}
			}

			{ // inclusive index range
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listReports",
					"params": {"application": "%v", "from": "0x2", "to": "0x4"},
					"id": 0
				}`, numberToName(app)))

				resp := testRPCResponse[[]Result]{}
				require.NoError(t, json.Unmarshal(body, &resp))
				require.Len(t, resp.Result.Data, 3)
				for i, expected := range []uint64{2, 3, 4} {
					assert.Equal(t, expected, uint64(resp.Result.Data[i].Index))
				}
			}
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getTournament
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getTournament", func(t *testing.T) {
		method := getName(t.Name())

		// failure: tournament address not hex encoded -> invalid param
		t.Run("malformedAddressParameter", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			app := uint64(1)
			address := uint64(0)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getTournament",
				"params": {
				"application": "%v",
				"address": "%v"
				},
				"id": 0
				}`, numberToName(app), address))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Equal(t, "Invalid tournament address: invalid address '0'", resp.Error.Message)
		})

		// failure: tournament address not in database -> absent tournament
		t.Run("absent", func(t *testing.T) {
			var err error
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(3)
			enr := uint64(1)
			tnr := uint64(1)   // correct (register)
			wrong := uint64(2) // incorrect (query)
			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(enr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())
			err = s.repository.CreateTournament(ctx, numberToName(app),
				repotest.NewTournamentBuilder(appID).
					WithEpochIndex(enr).
					WithAddress(common.HexToAddress(hexutil.EncodeUint64(tnr))).
					Build())
			require.NoError(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, enr)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getTournament",
				"params": {
				"application": "%v",
				"address": "0x%040x"
				},
				"id": 0
				}`, numberToName(app), wrong))

			resp := testRPCResponse[*model.Tournament]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Tournament not found", resp.Error.Message)
		})

		// failure: application not in the database -> application not found
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(0xdeadbeef)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getTournament",
				"params": {
				"application": "%v",
				"address": "0x%040x"
				},
				"id": 0
				}`, numberToName(nr), 0))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: tournament present in the database -> retrieve tournament
		t.Run("success", func(t *testing.T) {
			var err error
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(3)
			enr := uint64(1)
			tnr := uint64(1)
			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(enr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())
			err = s.repository.CreateTournament(ctx, numberToName(app),
				repotest.NewTournamentBuilder(appID).
					WithEpochIndex(enr).
					WithAddress(common.HexToAddress(hexutil.EncodeUint64(tnr))).
					Build())
			require.NoError(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, enr)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getTournament",
				"params": {
				"application": "%v",
				"address": "0x%040x"
				},
				"id": 0
				}`, numberToName(app), tnr))

			type Result struct {
				EpochIndex              hex64           `json:"epoch_index"`
				Address                 common.Address  `json:"address"`
				ParentTournamentAddress *common.Address `json:"parent_tournament_address"`
				ParentMatchIDHash       *common.Hash    `json:"parent_match_id_hash"`
				MaxLevel                hex64           `json:"max_level"`
				Level                   hex64           `json:"level"`
				Log2Step                hex64           `json:"log2step"`
				Height                  hex64           `json:"height"`
				WinnerCommitment        *common.Hash    `json:"winner_commitment"`
				FinalStateHash          *common.Hash    `json:"final_state_hash"`
				FinishedAtBlock         hex64           `json:"finished_at_block"`
				CreatedAt               time.Time       `json:"created_at"`
				UpdatedAt               time.Time       `json:"updated_at"`
			}

			resp := testRPCResponse[Result]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, hex64(enr), resp.Result.Data.EpochIndex)
		})
	})

	////////////////////////////////////////////////////////////////////////
	// cartesi_listTournaments
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_listTournaments", func(t *testing.T) {
		method := getName(t.Name())

		// failure: no tournament in the database -> no application
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(1)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listTournaments",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Tournament]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: no tournament in the database -> 0
		t.Run("empty", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listTournaments",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Tournament]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, 0, len(resp.Result.Data))
		})

		// failure: parent_tournament_address not a valid address -> a
		// single invalid params error. The Unmarshal nil-check doubles as
		// a regression guard: the handler once kept going after writing
		// the error, producing two JSON bodies in one response, which
		// makes Unmarshal fail with a trailing-data error.
		t.Run("invalidParentTournamentAddress", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listTournaments",
				"params": {
				"application": "%v",
				"parent_tournament_address": "0xnothex"
				},
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Contains(t, resp.Error.Message, "Invalid parent tournament address")
		})

		// failure: parent_match_id_hash not a valid hash -> a single
		// invalid params error (same regression guard as above).
		t.Run("invalidParentMatchIdHash", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listTournaments",
				"params": {
				"application": "%v",
				"parent_match_id_hash": "0xnothex"
				},
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Contains(t, resp.Error.Message, "Invalid parent match ID hash")
		})

		// success: many tournaments
		// create one application with many (epoch, tournament) pairs.
		// use their shared Index / EpochIndex to check pagination.
		t.Run("many", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(3)
			appID := s.newTestApplication(ctx, t, app)

			many := uint64(100)
			limit := uint64(many / 2)
			for tnr := range many {
				s.createTestEpoch(ctx, t, numberToName(app),
					repotest.NewEpochBuilder(appID).
						WithIndex(tnr).
						WithStatus(model.EpochStatus_ClaimAccepted).
						Build())
				err := s.repository.CreateTournament(ctx, numberToName(app),
					repotest.NewTournamentBuilder(appID).
						WithEpochIndex(tnr).
						WithAddress(common.HexToAddress(hexutil.EncodeUint64(tnr))).
						Build())
				require.NoError(t, err, "on test case: %v, application: %v, index: %v", 0, appID, tnr)
			}

			{ // offset == 0, descending = false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listTournaments",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 0, false))

				resp := testRPCResponse[[]listTournamentsResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i
					assert.Equal(t, nr, uint64(resp.Result.Data[i].EpochIndex))
				}
			}

			{ // offset == 1, descending == false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listTournaments",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 1, false))

				resp := testRPCResponse[[]listTournamentsResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i + 1
					assert.Equal(t, nr, uint64(resp.Result.Data[i].EpochIndex))
				}
			}

			{ // offset == 0, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listTournaments",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 0, true))

				resp := testRPCResponse[[]listTournamentsResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 1
					assert.Equal(t, nr, uint64(resp.Result.Data[i].EpochIndex))
				}
			}

			{ // offset == 1, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listTournaments",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 1, true))

				resp := testRPCResponse[[]listTournamentsResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 2
					assert.Equal(t, nr, uint64(resp.Result.Data[i].EpochIndex))
				}
			}
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getCommitment
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getCommitment", func(t *testing.T) {
		method := getName(t.Name())

		// failure: epoch_index not hex encoded -> invalid param
		t.Run("malformedEpochIndex", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			app := uint64(1)
			nr := uint64(0)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getCommitment",
				"params": {
				"application": "%v",
				"epoch_index": "%v"
				},
				"id": 0
				}`, numberToName(app), nr))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Equal(t, "Invalid epoch index: expected hex encoded value", resp.Error.Message)
		})

		// failure: commitment not in the database -> resource not found
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			nr := uint64(0)

			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(nr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getCommitment",
				"params": {
				"application": "%v",
				"epoch_index": "%v",
				"tournament_address": "0x%020x",
				"commitment": "0x%064x"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(nr+1), 0, 0))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Commitment not found", resp.Error.Message)
		})

		// failure: application not in the database -> application not found
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(0xdeadbeef)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getCommitment",
				"params": {
				"application": "%v",
				"epoch_index": "%v",
				"tournament_address": "0x%020x",
				"commitment": "0x%064x"
				},
				"id": 0
				}`, numberToName(nr), hexutil.EncodeUint64(0), 0, 0))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: commitment is in the database -> retrieve epoch
		t.Run("present", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			nr := uint64(1)
			address := common.HexToAddress("0x01")
			commitment := common.HexToHash("0xdeadbeef")

			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(nr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			err := s.repository.CreateTournament(ctx, numberToName(app),
				repotest.NewTournamentBuilder(appID).
					WithEpochIndex(nr).
					WithAddress(address).
					Build())
			require.NoError(t, err, "failed to create tournament. on test case: %v, application: %v, epoch_index: %v", 0, appID, nr)

			err = s.repository.CreateCommitment(ctx, numberToName(app),
				repotest.NewCommitmentBuilder(appID).
					WithEpochIndex(nr).
					WithTournamentAddress(address).
					WithCommitmentHash(commitment).
					Build())
			require.NoError(t, err, "failed to create commitment. on test case: %v, application: %v, epoch_index: %v", 0, appID, nr)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getCommitment",
				"params": {
				"application": "%v",
				"epoch_index": "%v",
				"tournament_address": "%v",
				"commitment": "0x%064x"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(nr), address, commitment))

			resp := testRPCResponse[getCommitmentResult]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, hex64(nr), resp.Result.Data.EpochIndex)
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getMatch
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getMatch", func(t *testing.T) {
		method := getName(t.Name())

		// failure: epoch_index not hex encoded -> invalid param
		t.Run("malformedEpochIndex", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			app := uint64(1)
			nr := uint64(0)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getMatch",
				"params": {
				"application": "%v",
				"epoch_index": "%v"
				},
				"id": 0
				}`, numberToName(app), nr))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Equal(t, "Invalid epoch index: expected hex encoded value", resp.Error.Message)
		})

		// failure: epoch not in the database -> resource not found
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			nr := uint64(0)

			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(nr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getMatch",
				"params": {
				"application": "%v",
				"epoch_index": "%v",
				"tournament_address": "0x%020x",
				"id_hash": "0x%064x"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(nr+1), 0, 0))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Match not found", resp.Error.Message)
		})

		// failure: application not in the database -> application not found
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(0xdeadbeef)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getMatch",
				"params": {
				"application": "%v",
				"epoch_index": "%v",
				"tournament_address": "0x%020x",
				"id_hash": "0x%064x"
				},
				"id": 0
				}`, numberToName(nr), hexutil.EncodeUint64(0), 0, 0))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: commitment is in the database -> retrieve epoch
		t.Run("present", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			nr := uint64(2)
			address := common.HexToAddress("0x03")
			idHash := common.HexToHash("0x04")

			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(nr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			err := s.repository.CreateTournament(ctx, numberToName(app),
				repotest.NewTournamentBuilder(appID).
					WithEpochIndex(nr).
					WithAddress(address).
					Build())
			require.NoError(t, err, "failed to create tournament. on test case: %v, application: %v, epoch_index: %v", 0, appID, nr)

			commitment := repotest.NewCommitmentBuilder(appID).
				WithEpochIndex(nr).
				WithTournamentAddress(address).
				Build()
			err = s.repository.CreateCommitment(ctx, numberToName(app), commitment)
			require.NoError(t, err, "failed to create commitment. on test case: %v, application: %v, epoch_index: %v", 0, appID, nr)

			err = s.repository.CreateMatch(ctx, numberToName(app),
				repotest.NewMatchBuilder(appID).
					WithEpochIndex(nr).
					WithTournamentAddress(address).
					WithIDHash(idHash).
					WithCommitmentOne(commitment.Commitment).
					WithCommitmentTwo(commitment.Commitment).
					WithWinner(model.WinnerCommitment_NONE).
					WithDeletionReason(model.MatchDeletionReason_TIMEOUT).
					Build())
			require.NoError(t, err, "failed to create match. on test case: %v, application: %v, epoch_index: %v", 0, appID, nr)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getMatch",
				"params": {
				"application": "%v",
				"epoch_index": "%v",
				"tournament_address": "0x%020x",
				"id_hash": "%v"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(nr), address, idHash))

			resp := testRPCResponse[getMatchResult]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, idHash, resp.Result.Data.IDHash)
			assert.Equal(t, hex64(nr), resp.Result.Data.EpochIndex)
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getMatchAdvance
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getMatchAdvance", func(t *testing.T) {
		method := getName(t.Name())

		// failure: epoch_index not hex encoded -> invalid param
		t.Run("malformedEpochIndex", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			app := uint64(1)
			nr := uint64(0)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getMatchAdvance",
				"params": {
				"application": "%v",
				"epoch_index": "%v"
				},
				"id": 0
				}`, numberToName(app), nr))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
			assert.Equal(t, "Invalid epoch index: expected hex encoded value", resp.Error.Message)
		})

		// failure: epoch not in the database -> resource not found
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			nr := uint64(0)

			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(nr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getMatchAdvance",
				"params": {
				"application": "%v",
				"epoch_index": "0x%020x",
				"tournament_address": "0x%040x",
				"id_hash": "0x%064x",
				"parent": "0x%064x"
				},
				"id": 0
				}`, numberToName(app), nr+1, 0, 0, 0))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Match advanced not found", resp.Error.Message)
		})

		// failure: application not in the database -> application not found
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(0xdeadbeef)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getMatchAdvance",
				"params": {
				"application": "%v",
				"epoch_index": "0x%020x",
				"tournament_address": "0x%040x",
				"id_hash": "0x%064x",
				"parent": "0x%064x"
				},
				"id": 0
				}`, numberToName(nr), 0, 0, 0, 0))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: commitment is in the database -> retrieve epoch
		t.Run("present", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			nr := uint64(2)
			address := common.HexToAddress("0x03")
			idHash := common.HexToHash("0x04")
			parentHex := "0xAbCdEf0123456789aBcDeF0123456789AbCdEf0123456789aBcDeF0123456789"
			parent := common.HexToHash(parentHex)

			appID := s.newTestApplication(ctx, t, app)
			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(nr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			err := s.repository.CreateTournament(ctx, numberToName(app),
				repotest.NewTournamentBuilder(appID).
					WithEpochIndex(nr).
					WithAddress(address).
					Build())
			require.NoError(t, err, "failed to create tournament. on test case: %v, application: %v, epoch_index: %v", 0, appID, nr)

			commitment := repotest.NewCommitmentBuilder(appID).
				WithEpochIndex(nr).
				WithTournamentAddress(address).
				Build()
			err = s.repository.CreateCommitment(ctx, numberToName(app), commitment)
			require.NoError(t, err, "failed to create commitment. on test case: %v, application: %v, epoch_index: %v", 0, appID, nr)

			err = s.repository.CreateMatch(ctx, numberToName(app),
				repotest.NewMatchBuilder(appID).
					WithEpochIndex(nr).
					WithTournamentAddress(address).
					WithIDHash(idHash).
					WithCommitmentOne(commitment.Commitment).
					WithCommitmentTwo(commitment.Commitment).
					WithWinner(model.WinnerCommitment_NONE).
					WithDeletionReason(model.MatchDeletionReason_TIMEOUT).
					Build())
			require.NoError(t, err, "failed to create match. on test case: %v, application: %v, epoch_index: %v", 0, appID, nr)

			err = s.repository.CreateMatchAdvanced(ctx, numberToName(app),
				repotest.NewMatchAdvancedBuilder(appID).
					WithEpochIndex(nr).
					WithTournamentAddress(address).
					WithIDHash(idHash).
					WithOtherParent(parent).
					Build())
			require.NoError(t, err, "failed to create match advanced. on test case: %v, application: %v, epoch_index: %v", 0, appID, nr)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getMatchAdvance",
				"params": {
				"application": "%v",
				"epoch_index": "0x%020x",
				"tournament_address": "0x%020x",
				"id_hash": "0x%064x",
				"parent": "%s"
				},
				"id": 0
				}`, numberToName(app), nr, address, idHash, parentHex))

			resp := testRPCResponse[getMatchAdvancedResult]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, idHash, resp.Result.Data.IDHash)
			assert.Equal(t, hex64(nr), resp.Result.Data.EpochIndex)
		})
	})

	////////////////////////////////////////////////////////////////////////
	// cartesi_listMatches
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_listMatches", func(t *testing.T) {
		method := getName(t.Name())

		// failure: no match in the database -> no application
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(1)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listMatches",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Match]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: no match in the database -> 0
		t.Run("empty", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listMatches",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Match]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, 0, len(resp.Result.Data))
		})

		// success: many matches
		// create one application with many (epoch, matches) pairs.
		// use their shared EpochIndex to check pagination.
		t.Run("many", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(3)
			appID := s.newTestApplication(ctx, t, app)

			many := uint64(100)
			limit := uint64(many / 2)
			for tnr := range many {
				// populate the database with stub data
				address := common.HexToAddress(hexutil.EncodeUint64(tnr))
				idHash := common.HexToHash(hexutil.EncodeUint64(tnr))
				commitmentOne := common.HexToHash(hexutil.EncodeUint64(tnr))
				commitmentTwo := common.HexToHash(hexutil.EncodeUint64(tnr))

				s.createTestEpoch(ctx, t, numberToName(app),
					repotest.NewEpochBuilder(appID).
						WithIndex(tnr).
						WithStatus(model.EpochStatus_ClaimAccepted).
						Build())

				err := s.repository.CreateTournament(ctx, numberToName(app),
					repotest.NewTournamentBuilder(appID).
						WithEpochIndex(tnr).
						WithAddress(common.HexToAddress(hexutil.EncodeUint64(tnr))).
						Build())
				require.NoError(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, tnr)

				err = s.repository.CreateCommitment(ctx, numberToName(app),
					repotest.NewCommitmentBuilder(appID).
						WithEpochIndex(tnr).
						WithTournamentAddress(address).
						WithCommitmentHash(commitmentOne).
						Build())
				require.NoError(t, err, "failed to create commitment. on test case: %v, application: %v, epoch_index: %v", 0, appID, tnr)

				err = s.repository.CreateMatch(ctx, numberToName(app),
					repotest.NewMatchBuilder(appID).
						WithEpochIndex(tnr).
						WithTournamentAddress(address).
						WithIDHash(idHash).
						WithCommitmentOne(commitmentOne).
						WithCommitmentTwo(commitmentTwo).
						WithWinner(model.WinnerCommitment_NONE).
						WithDeletionReason(model.MatchDeletionReason_TIMEOUT).
						Build())
				require.NoError(t, err, "on test case: %v, application: %v, report_index: %v", 0, appID, tnr)
			}

			{ // offset == 0, descending = false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listMatches",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 0, false))

				resp := testRPCResponse[[]listMatchesResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i
					assert.Equal(t, nr, uint64(resp.Result.Data[i].EpochIndex))
				}
			}

			{ // offset == 1, descending == false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listMatches",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 1, false))

				resp := testRPCResponse[[]listMatchesResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i + 1
					assert.Equal(t, nr, uint64(resp.Result.Data[i].EpochIndex))
				}
			}

			{ // offset == 0, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listMatches",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 0, true))

				resp := testRPCResponse[[]listMatchesResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 1
					assert.Equal(t, nr, uint64(resp.Result.Data[i].EpochIndex))
				}
			}

			{ // offset == 1, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listMatches",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 1, true))

				resp := testRPCResponse[[]listMatchesResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 2
					assert.Equal(t, nr, uint64(resp.Result.Data[i].EpochIndex))
				}
			}
		})
	})

	////////////////////////////////////////////////////////////////////////
	// cartesi_listMatchAdvances
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_listMatchAdvances", func(t *testing.T) {
		method := getName(t.Name())

		// failure: no match advance in the database -> no application
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			app := uint64(0)
			nr := uint64(1)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listMatchAdvances",
				"params": {
				"application": "%v",
				"epoch_index": "%v",
				"tournament_address": "0x%020x",
				"id_hash": "0x%064x"
				},
				"id": 0
				}`, numberToName(app+1), hexutil.EncodeUint64(nr), 0, 0))

			resp := testRPCResponse[[]listMatchesResult]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: no tournament in the database -> 0
		t.Run("empty", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(0)
			nr := uint64(1)
			s.newTestApplication(ctx, t, app)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listMatchAdvances",
				"params": {
				"application": "%v",
				"epoch_index": "%v",
				"tournament_address": "0x%020x",
				"id_hash": "0x%064x"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(nr), 0, 0))

			resp := testRPCResponse[[]listMatchesResult]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, 0, len(resp.Result.Data))
		})

		// success: many match_advances
		// create one tournament with many (match_advances) pairs.
		t.Run("many", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			appID := s.newTestApplication(ctx, t, app)
			enr := uint64(2)
			tournamentAddress := common.HexToAddress(hexutil.EncodeUint64(enr))
			commitment := common.HexToHash(hexutil.EncodeUint64(enr))
			idHash := common.HexToHash(hexutil.EncodeUint64(enr))

			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(enr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			err := s.repository.CreateTournament(ctx, numberToName(app),
				repotest.NewTournamentBuilder(appID).
					WithEpochIndex(enr).
					WithAddress(tournamentAddress).
					Build())
			require.NoError(t, err)

			err = s.repository.CreateCommitment(ctx, numberToName(app),
				repotest.NewCommitmentBuilder(appID).
					WithEpochIndex(enr).
					WithTournamentAddress(tournamentAddress).
					WithCommitmentHash(commitment).
					Build())
			require.NoError(t, err)

			err = s.repository.CreateMatch(ctx, numberToName(app),
				repotest.NewMatchBuilder(appID).
					WithEpochIndex(enr).
					WithTournamentAddress(tournamentAddress).
					WithIDHash(idHash).
					WithCommitmentOne(commitment).
					WithCommitmentTwo(commitment).
					WithWinner(model.WinnerCommitment_NONE).
					WithDeletionReason(model.MatchDeletionReason_NOT_DELETED).
					Build())
			require.NoError(t, err)

			many := uint64(100)
			limit := uint64(many / 2)
			for nr := range many {
				otherParent := common.HexToHash(hexutil.EncodeUint64(nr))
				err = s.repository.CreateMatchAdvanced(ctx, numberToName(app),
					repotest.NewMatchAdvancedBuilder(appID).
						WithEpochIndex(enr).
						WithTournamentAddress(tournamentAddress).
						WithIDHash(idHash).
						WithOtherParent(otherParent).
						Build())
				require.NoError(t, err, "failed to create match advance, app: %v, nr: %v", app, nr)
			}

			{ // offset == 0, descending = false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listMatchAdvances",
					"params": {
					"application": "%v",
					"epoch_index": "0x%020x",
					"tournament_address": "0x%020x",
					"id_hash": "0x%064x",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), enr, tournamentAddress, idHash, limit, 0, false))

				resp := testRPCResponse[[]listMatchAdvancedResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i
					assert.Equal(t, common.HexToHash(hexutil.EncodeUint64(nr)), resp.Result.Data[i].OtherParent)
				}
			}

			{ // offset == 1, descending == false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listMatchAdvances",
					"params": {
					"application": "%v",
					"epoch_index": "0x%020x",
					"tournament_address": "0x%020x",
					"id_hash": "0x%064x",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), enr, tournamentAddress, idHash, limit, 1, false))

				resp := testRPCResponse[[]listMatchAdvancedResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i + 1
					assert.Equal(t, common.HexToHash(hexutil.EncodeUint64(nr)), resp.Result.Data[i].OtherParent)
				}
			}

			{ // offset == 0, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listMatchAdvances",
					"params": {
					"application": "%v",
					"epoch_index": "0x%020x",
					"tournament_address": "0x%020x",
					"id_hash": "0x%064x",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), enr, tournamentAddress, idHash, limit, 0, true))

				resp := testRPCResponse[[]listMatchAdvancedResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 1
					assert.Equal(t, common.HexToHash(hexutil.EncodeUint64(nr)), resp.Result.Data[i].OtherParent)
				}
			}

			{ // offset == 1, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listMatchAdvances",
					"params": {
					"application": "%v",
					"epoch_index": "0x%020x",
					"tournament_address": "0x%020x",
					"id_hash": "0x%064x",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), enr, tournamentAddress, idHash, limit, 1, true))

				resp := testRPCResponse[[]listMatchAdvancedResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 2
					assert.Equal(t, common.HexToHash(hexutil.EncodeUint64(nr)), resp.Result.Data[i].OtherParent)
				}
			}
		})
	})

	////////////////////////////////////////////////////////////////////////
	// cartesi_listCommitments
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_listCommitments", func(t *testing.T) {
		method := getName(t.Name())

		// failure: no commitment in the database -> no application
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			app := uint64(0)
			nr := uint64(1)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listCommitments",
				"params": {
				"application": "%v",
				"epoch_index": "%v",
				"tournament_address": "0x%020x",
				"id_hash": "0x%064x"
				},
				"id": 0
				}`, numberToName(app+1), hexutil.EncodeUint64(nr), 0, 0))

			resp := testRPCResponse[[]listCommitmentResult]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: no commitment in the database -> 0
		t.Run("empty", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listCommitments",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]listCommitmentResult]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, 0, len(resp.Result.Data))
		})

		// success: many commitments
		// create one tournament with many commitments.
		t.Run("many", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			appID := s.newTestApplication(ctx, t, app)
			enr := uint64(2)
			tournamentAddress := common.HexToAddress(hexutil.EncodeUint64(enr))

			s.createTestEpoch(ctx, t, numberToName(app),
				repotest.NewEpochBuilder(appID).
					WithIndex(enr).
					WithStatus(model.EpochStatus_ClaimAccepted).
					Build())

			err := s.repository.CreateTournament(ctx, numberToName(app),
				repotest.NewTournamentBuilder(appID).
					WithEpochIndex(enr).
					WithAddress(tournamentAddress).
					Build())
			require.NoError(t, err)

			many := uint64(100)
			limit := uint64(many / 2)
			for nr := range many {
				commitment := common.HexToHash(hexutil.EncodeUint64(nr))

				err = s.repository.CreateCommitment(ctx, numberToName(app),
					repotest.NewCommitmentBuilder(appID).
						WithEpochIndex(enr).
						WithTournamentAddress(tournamentAddress).
						WithCommitmentHash(commitment).
						Build())
				require.NoError(t, err)
			}

			{ // offset == 0, descending = false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listCommitments",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 0, false))

				resp := testRPCResponse[[]listCommitmentResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i
					assert.Equal(t, common.HexToHash(hexutil.EncodeUint64(nr)), resp.Result.Data[i].Commitment)
				}
			}

			{ // offset == 1, descending == false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listCommitments",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 1, false))

				resp := testRPCResponse[[]listCommitmentResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := i + 1
					assert.Equal(t, common.HexToHash(hexutil.EncodeUint64(nr)), resp.Result.Data[i].Commitment)
				}
			}

			{ // offset == 0, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listCommitments",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 0, true))

				resp := testRPCResponse[[]listCommitmentResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 1
					assert.Equal(t, common.HexToHash(hexutil.EncodeUint64(nr)), resp.Result.Data[i].Commitment)
				}
			}

			{ // offset == 1, descending = true
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listCommitments",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 1, true))

				resp := testRPCResponse[[]listCommitmentResult]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					nr := many - i - 2
					assert.Equal(t, common.HexToHash(hexutil.EncodeUint64(nr)), resp.Result.Data[i].Commitment)
				}
			}
		})
	})

	////////////////////////////////////////////////////////////////////////
	// getWithdrawal
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_getWithdrawal", func(t *testing.T) {
		method := getName(t.Name())

		// failure: account_index not hex encoded -> invalid params
		t.Run("malformed", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			app := uint64(1)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getWithdrawal",
				"params": {
				"application": "%v",
				"account_index": "%v"
				},
				"id": 0
				}`, numberToName(app), "not-hex"))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
		})

		// failure: application missing -> application not found.
		// GetWithdrawal's joined SELECT returns (nil, nil) for either
		// missing application or missing account_index; the handler
		// disambiguates by checking application existence, so an unknown
		// application reports "Application not found".
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			app := uint64(2)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getWithdrawal",
				"params": {
				"application": "%v",
				"account_index": "%v"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(0)))

			resp := testRPCResponse[*model.Withdrawal]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// failure: application exists but no matching account_index ->
		// resource not found.
		t.Run("absentAccountIndex", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(3)
			s.newTestApplication(ctx, t, app)

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getWithdrawal",
				"params": {
				"application": "%v",
				"account_index": "%v"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(99)))

			resp := testRPCResponse[*model.Withdrawal]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Withdrawal not found", resp.Error.Message)
		})

		// success: account_index in DB -> return the row.
		t.Run("success", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(4)
			appID := s.newTestApplication(ctx, t, app)
			w := &model.Withdrawal{
				ApplicationID:   appID,
				AccountIndex:    7,
				Account:         []byte{0xaa, 0xbb},
				Output:          []byte{0xcc, 0xdd},
				BlockNumber:     1234,
				TransactionHash: common.HexToHash("0xcafe"),
				LogIndex:        2,
			}
			require.NoError(t, s.repository.InsertWithdrawal(ctx, w))

			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_getWithdrawal",
				"params": {
				"application": "%v",
				"account_index": "%v"
				},
				"id": 0
				}`, numberToName(app), hexutil.EncodeUint64(w.AccountIndex)))

			type Result struct {
				AccountIndex    hex64       `json:"account_index"`
				Account         string      `json:"account"`
				Output          string      `json:"output"`
				BlockNumber     hex64       `json:"block_number"`
				TransactionHash common.Hash `json:"transaction_hash"`
				LogIndex        hex64       `json:"log_index"`
			}
			resp := testRPCResponse[Result]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Nil(t, resp.Error)
			assert.Equal(t, w.AccountIndex, uint64(resp.Result.Data.AccountIndex))
			assert.Equal(t, "0x"+common.Bytes2Hex(w.Account), resp.Result.Data.Account)
			assert.Equal(t, "0x"+common.Bytes2Hex(w.Output), resp.Result.Data.Output)
			assert.Equal(t, w.BlockNumber, uint64(resp.Result.Data.BlockNumber))
			assert.Equal(t, w.TransactionHash, resp.Result.Data.TransactionHash)
			assert.Equal(t, uint64(w.LogIndex), uint64(resp.Result.Data.LogIndex))
		})
	})

	////////////////////////////////////////////////////////////////////////
	// listWithdrawals
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_listWithdrawals", func(t *testing.T) {
		method := getName(t.Name())

		// failure: application missing -> resource not found
		t.Run("absentApplication", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())

			nr := uint64(1)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listWithdrawals",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Withdrawal]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_APPLICATION_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: application present but no withdrawals -> empty list
		t.Run("empty", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, nr)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listWithdrawals",
				"params": { "application": "%v" },
				"id": 0
				}`, numberToName(nr)))

			resp := testRPCResponse[[]model.Withdrawal]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Nil(t, resp.Error)
			assert.Equal(t, 0, len(resp.Result.Data))
		})

		// failure: malformed account_index filter -> invalid params
		t.Run("malformedAccountIndex", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(2)
			s.newTestApplication(ctx, t, app)
			body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
				"jsonrpc": "2.0",
				"method": "cartesi_listWithdrawals",
				"params": {
				"application": "%v",
				"account_index": "%v"
				},
				"id": 0
				}`, numberToName(app), "not-hex"))

			resp := testRPCResponse[any]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, JSONRPC_INVALID_PARAMS, resp.Error.Code)
		})

		// success: many withdrawals, ascending + descending + pagination + filter
		t.Run("many", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(3)
			appID := s.newTestApplication(ctx, t, app)

			const many = uint64(10)
			const limit = uint64(many / 2)
			for i := uint64(0); i < many; i++ {
				require.NoError(t, s.repository.InsertWithdrawal(ctx, &model.Withdrawal{
					ApplicationID:   appID,
					AccountIndex:    i,
					Account:         []byte{0xaa, byte(i)},
					Output:          []byte{0xbb, byte(i)},
					BlockNumber:     1000 + i,
					TransactionHash: common.HexToHash(hexutil.EncodeUint64(i)),
					LogIndex:        uint(i % 4),
				}))
			}

			type Result struct {
				AccountIndex hex64 `json:"account_index"`
			}

			{ // offset == 0, descending = false → ascending account_index 0..limit-1
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listWithdrawals",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 0, false))

				resp := testRPCResponse[[]Result]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					assert.Equal(t, i, uint64(resp.Result.Data[i].AccountIndex))
				}
			}

			{ // offset == 1, descending == false
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listWithdrawals",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 1, false))

				resp := testRPCResponse[[]Result]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					assert.Equal(t, i+1, uint64(resp.Result.Data[i].AccountIndex))
				}
			}

			{ // offset == 0, descending = true → last index first
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listWithdrawals",
					"params": {
					"application": "%v",
					"limit": %v,
					"offset": %v,
					"descending": %v
					},
					"id": 0
					}`, numberToName(app), limit, 0, true))

				resp := testRPCResponse[[]Result]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, limit, uint64(len(resp.Result.Data)))
				for i := range limit {
					assert.Equal(t, many-i-1, uint64(resp.Result.Data[i].AccountIndex))
				}
			}

			{ // account_index filter → exactly one row
				body := s.doRequest(t, 0, fmt.Appendf([]byte{}, `{
					"jsonrpc": "2.0",
					"method": "cartesi_listWithdrawals",
					"params": {
					"application": "%v",
					"account_index": "%v"
					},
					"id": 0
					}`, numberToName(app), hexutil.EncodeUint64(3)))

				resp := testRPCResponse[[]Result]{}
				assert.Nil(t, json.Unmarshal(body, &resp))
				assert.Equal(t, 1, len(resp.Result.Data))
				assert.Equal(t, uint64(3), uint64(resp.Result.Data[0].AccountIndex))
			}
		})
	})

	// tested methods, implemented methods and discover methods must match:
	data, err := discoverSpec.ReadFile("jsonrpc-discover.json")
	require.NoError(t, err)

	var schema jsonrpcSchema
	err = json.Unmarshal(data, &schema)
	require.NoError(t, err)

	allMethods := make(map[string]bool)
	tested := make(map[string]bool)
	for k := range testHistogram {
		allMethods[k] = true
		tested[k] = true
	}

	implemented := make(map[string]bool)
	for k := range jsonrpcHandlers {
		allMethods[k] = true
		implemented[k] = true
	}

	specified := make(map[string]bool)
	for _, v := range schema.Methods {
		allMethods[v.Name] = true
		specified[v.Name] = true
	}

	// Check each method
	var errors []string
	for method := range allMethods {
		hasTest := tested[method]
		hasImpl := implemented[method]
		hasSpec := specified[method]

		// All methods must be tested and implemented
		// rpc.discover is not discovered (not in schema), others must be
		expectedInSpec := method != "rpc.discover"

		var missing []string
		if !hasTest {
			missing = append(missing, "tests")
		}
		if !hasImpl {
			missing = append(missing, "implementation")
		}
		if hasSpec != expectedInSpec {
			if expectedInSpec {
				missing = append(missing, "specification")
			} else {
				missing = append(missing, "should not be in specification")
			}
		}
		if len(missing) > 0 {
			errors = append(errors, fmt.Sprintf("Method %s is missing: %v", method, missing))
		}
	}

	if len(errors) > 0 {
		t.Errorf("Method coverage issues:\n%s", strings.Join(errors, "\n"))
	}
}

func TestListIndexRangeValidation(t *testing.T) {
	for _, method := range []string{
		"cartesi_listEpochs",
		"cartesi_listInputs",
		"cartesi_listOutputs",
		"cartesi_listReports",
	} {
		t.Run(method, func(t *testing.T) {
			s := newBatchTestService()
			body := []byte(fmt.Sprintf(`{
				"jsonrpc":"2.0",
				"method":%q,
				"params":{"application":"app","from":"0x2","to":"0x1"},
				"id":1
			}`, method))
			rr := serveRPC(t, s, body)

			require.Equal(t, http.StatusOK, rr.Code)
			response := decodeRPCResponse(t, rr.Body.Bytes())
			requireRPCError(t, response, float64(1), JSONRPC_INVALID_PARAMS)
			require.Equal(t, "invalid index range: from must be less than or equal to to", response.Error.Message)
		})
	}
}

func TestParseIndexRange(t *testing.T) {
	from := "0x2"
	to := "0x4"
	indexRange, err := parseIndexRange(&from, &to)
	require.NoError(t, err)
	require.Equal(t, repository.Range{Start: 2, End: 4}, *indexRange)

	indexRange, err = parseIndexRange(&from, nil)
	require.NoError(t, err)
	require.Equal(t, uint64(2), indexRange.Start)
	require.Equal(t, uint64(math.MaxUint64), indexRange.End)

	indexRange, err = parseIndexRange(nil, &to)
	require.NoError(t, err)
	require.Equal(t, uint64(0), indexRange.Start)
	require.Equal(t, uint64(4), indexRange.End)

	invalid := "2"
	_, err = parseIndexRange(&invalid, nil)
	require.EqualError(t, err, "invalid from index: expected hex encoded value")
	_, err = parseIndexRange(nil, &invalid)
	require.EqualError(t, err, "invalid to index: expected hex encoded value")
}

func TestRequestMethodIsInfoLogged(t *testing.T) {
	s := newBatchTestService()
	var logs bytes.Buffer
	s.Logger = slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))

	const method = "attacker_controlled_method"
	serveRPC(t, s, []byte(`{"jsonrpc":"2.0","method":"attacker_controlled_method","id":1}`))

	var methodLogs int
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["method"] == method {
			require.Equal(t, "INFO", record["level"])
			methodLogs++
		}
	}
	require.Positive(t, methodLogs)
}
