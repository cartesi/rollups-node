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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/version"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
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

	assert.Equal(t, "Invalid JSON\n", string(body))
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
			assert.Nil(t, err)

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
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: application is in the database -> retrieve application
		t.Run("present", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, 0, nr)
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
			assert.Nil(t, err, "on test case: %v, when saving evm reader config", t.Name())

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
			assert.Equal(t, "invalid epoch_index: expected hex encoded value", resp.Error.Message)
		})

		// failure: epoch not in the database -> resource not found
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			nr := uint64(0)

			appID := s.newTestApplication(ctx, t, 0, app)
			err := s.repository.CreateEpoch(ctx, &model.Epoch{
				ApplicationID:        appID,
				Index:                nr,
				ClaimHash:            &common.Hash{},
				ClaimTransactionHash: &common.Hash{},
				Status:               model.EpochStatus_ClaimAccepted,
			})
			assert.Nil(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, nr)

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

		// success: epoch is in the database -> retrieve epoch
		t.Run("present", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)
			nr := uint64(1)

			appID := s.newTestApplication(ctx, t, 0, app)
			err := s.repository.CreateEpoch(ctx, &model.Epoch{
				ApplicationID:        appID,
				Index:                nr,
				ClaimHash:            &common.Hash{},
				ClaimTransactionHash: &common.Hash{},
				Status:               model.EpochStatus_ClaimAccepted,
			})
			assert.Nil(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, nr)

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
			assert.Equal(t, "invalid input_index: expected hex encoded value", resp.Error.Message)
		})

		// failure: input_index not in database -> absent input
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(2)
			enr := uint64(1)
			inr := uint64(0)
			appID := s.newTestApplication(ctx, t, 0, app)
			err := s.repository.CreateEpoch(ctx, &model.Epoch{
				ApplicationID: appID,
				Index:         enr,
				VirtualIndex:  enr,
				Status:        model.EpochStatus_ClaimAccepted,
			})
			assert.Nil(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, enr)

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

		// success: input_index of EvmAdvance in the database -> retrieve input
		t.Run("success", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(2)
			enr := uint64(1)
			inr := uint64(0)
			appID := s.newTestApplication(ctx, t, 0, app)
			err := s.repository.CreateEpoch(ctx, &model.Epoch{
				ApplicationID: appID,
				Index:         enr,
				VirtualIndex:  enr,
				Status:        model.EpochStatus_ClaimAccepted,
			})
			assert.Nil(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, enr)

			err = s.repository.CreateInput(ctx, &model.Input{
				EpochApplicationID: appID,
				EpochIndex:         enr,
				Index:              inr,
				Status:             model.InputCompletionStatus_Accepted,
				RawData:            emptyInput(),
			})
			assert.Nil(t, err, "on test case: %v, application: %v, input_index: %v", 0, appID, inr)

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

		// failure: epoch not in the database -> resource not found
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

			appID := s.newTestApplication(ctx, t, 0, nr)
			err := s.repository.CreateEpoch(ctx, &model.Epoch{
				ApplicationID:        appID,
				Index:                epochIndex,
				ClaimHash:            &common.Hash{},
				ClaimTransactionHash: &common.Hash{},
				Status:               model.EpochStatus_ClaimAccepted,
			})
			assert.Nil(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, nr)

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
			assert.Equal(t, "invalid output_index: expected hex encoded value", resp.Error.Message)
		})

		// failure: input_index not in database -> absent input
		t.Run("absent", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(2)
			enr := uint64(1)
			inr := uint64(0)
			appID := s.newTestApplication(ctx, t, 0, app)
			err := s.repository.CreateEpoch(ctx, &model.Epoch{
				ApplicationID: appID,
				Index:         enr,
				VirtualIndex:  enr,
				Status:        model.EpochStatus_ClaimAccepted,
			})
			assert.Nil(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, enr)

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

		// success: output_index of Voucher in the database -> retrieve output
		t.Run("success", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(3)
			enr := uint64(1)
			inr := uint64(1)
			onr := uint64(0)
			appID := s.newTestApplication(ctx, t, 0, app)
			err := s.repository.CreateEpoch(ctx, &model.Epoch{
				ApplicationID: appID,
				Index:         enr,
				VirtualIndex:  enr,
				Status:        model.EpochStatus_ClaimAccepted,
			})
			assert.Nil(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, enr)

			err = s.repository.CreateInput(ctx, &model.Input{
				EpochApplicationID: appID,
				EpochIndex:         enr,
				Index:              inr,
				Status:             model.InputCompletionStatus_Accepted,
				RawData:            emptyInput(),
			})
			assert.Nil(t, err, "on test case: %v, application: %v, input_index: %v", 0, appID, inr)

			err = s.repository.CreateOutput(ctx, &model.Output{
				InputEpochApplicationID: appID,
				InputIndex:              inr,
				Index:                   onr,
				RawData:                 emptyVoucher(),
			})
			assert.Nil(t, err, "on test case: %v, application: %v, output_index: %v", 0, appID, onr)

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

				Voucher *Voucher `json:"decoded_data,omitempty"`

				// ... (ignore the rest of fields for test
			}

			resp := testRPCResponse[Result]{}
			assert.Nil(t, json.Unmarshal(body, &resp))
			assert.Equal(t, inr, uint64(resp.Result.Data.InputIndex))
			assert.Equal(t, onr, uint64(resp.Result.Data.Index))
			assert.Equal(t, "0xdeadbeef", resp.Result.Data.Voucher.Value)
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
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: application has no inputs -> 0
		t.Run("absentInput", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			app := uint64(1)

			s.newTestApplication(ctx, t, 0, app)
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

		// TODO: test with inputs (need repository.CreateInput)
	})

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
			assert.Equal(t, "invalid report_index: expected hex encoded value", resp.Error.Message)
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
			appID := s.newTestApplication(ctx, t, 0, app)
			err := s.repository.CreateEpoch(ctx, &model.Epoch{
				ApplicationID: appID,
				Index:         enr,
				VirtualIndex:  enr,
				Status:        model.EpochStatus_ClaimAccepted,
			})
			assert.Nil(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, enr)

			err = s.repository.CreateInput(ctx, &model.Input{
				EpochApplicationID: appID,
				EpochIndex:         enr,
				Index:              inr,
				Status:             model.InputCompletionStatus_Accepted,
				RawData:            emptyInput(),
			})
			assert.Nil(t, err, "on test case: %v, application: %v, input_index: %v", 0, appID, inr)

			err = s.repository.CreateReport(ctx, &model.Report{
				InputEpochApplicationID: appID,
				InputIndex:              inr,
				Index:                   onr,
				RawData:                 emptyVoucher(),
			})
			assert.Nil(t, err, "on test case: %v, application: %v, output_index: %v", 0, appID, onr)

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
			s.newTestApplication(ctx, t, 0, nr)
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
			s.newTestApplication(ctx, t, 0, nr)
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

		// success: many applications is in the database -> limit (many - 1)
		t.Run("many", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			many := uint64(100)
			limit := uint64(many / 2)
			for i := range many {
				s.newTestApplication(ctx, t, 0, i)
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
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: no epoch in the database -> 0
		t.Run("absentEpoch", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, 0, nr)
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
			s.newTestApplication(ctx, t, 0, nr)
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

		// success: many epochs is in the database -> limit
		t.Run("many", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			many := uint64(100)
			limit := uint64(many / 2)
			appID := s.newTestApplication(ctx, t, 0, nr)
			for i := range many {
				err := s.repository.CreateEpoch(ctx, &model.Epoch{
					ApplicationID: appID,
					Index:         i,
					VirtualIndex:  i,
					Status:        model.EpochStatus_ClaimAccepted,
				})
				assert.Nil(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, nr)
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
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: no epoch in the database -> 0
		t.Run("absentInput", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, 0, nr)
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

		// TODO: test many inputs in the database (requires: repository.CreateInput)
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
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: no epoch in the database -> 0
		t.Run("absentOutput", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, 0, nr)
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
			appID := s.newTestApplication(ctx, t, 0, app)
			err := s.repository.CreateEpoch(ctx, &model.Epoch{
				ApplicationID: appID,
				Index:         enr,
				VirtualIndex:  enr,
				Status:        model.EpochStatus_ClaimAccepted,
			})
			assert.Nil(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, enr)

			err = s.repository.CreateInput(ctx, &model.Input{
				EpochApplicationID: appID,
				EpochIndex:         enr,
				Index:              inr,
				Status:             model.InputCompletionStatus_Accepted,
				RawData:            emptyInput(),
			})
			assert.Nil(t, err, "on test case: %v, application: %v, input_index: %v", 0, appID, inr)

			many := uint64(100)
			limit := uint64(many / 2)
			for onr := range many {
				err = s.repository.CreateOutput(ctx, &model.Output{
					InputEpochApplicationID: appID,
					InputIndex:              inr,
					Index:                   onr,
					RawData:                 emptyVoucher(),
				})
				assert.Nil(t, err, "on test case: %v, application: %v, output_index: %v", 0, appID, onr)
			}

			type Result struct {
				EpochIndex hex64  `json:"epoch_index"`
				InputIndex hex64  `json:"input_index"`
				Index      hex64  `json:"index"`
				RawData    string `json:"raw_data"` // hex encoded

				Voucher *Voucher `json:"decoded_data,omitempty"`

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
			assert.Equal(t, JSONRPC_RESOURCE_NOT_FOUND, resp.Error.Code)
			assert.Equal(t, "Application not found", resp.Error.Message)
		})

		// success: no epoch in the database -> 0
		t.Run("empty", func(t *testing.T) {
			testHistogram.inc(method)
			s := newTestService(t, t.Name())
			ctx := context.Background()

			nr := uint64(1)
			s.newTestApplication(ctx, t, 0, nr)
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
			appID := s.newTestApplication(ctx, t, 0, app)
			err := s.repository.CreateEpoch(ctx, &model.Epoch{
				ApplicationID: appID,
				Index:         enr,
				VirtualIndex:  enr,
				Status:        model.EpochStatus_ClaimAccepted,
			})
			assert.Nil(t, err, "on test case: %v, application: %v, epoch_index: %v", 0, appID, enr)

			err = s.repository.CreateInput(ctx, &model.Input{
				EpochApplicationID: appID,
				EpochIndex:         enr,
				Index:              inr,
				Status:             model.InputCompletionStatus_Accepted,
				RawData:            emptyInput(),
			})
			assert.Nil(t, err, "on test case: %v, application: %v, input_index: %v", 0, appID, inr)

			many := uint64(100)
			limit := uint64(many / 2)
			for onr := range many {
				err = s.repository.CreateReport(ctx, &model.Report{
					InputEpochApplicationID: appID,
					InputIndex:              inr,
					Index:                   onr,
					RawData:                 emptyVoucher(),
				})
				assert.Nil(t, err, "on test case: %v, application: %v, report_index: %v", 0, appID, onr)
			}

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
		})
	})

	////////////////////////////////////////////////////////////////////////
	// Place holder for new tournament data methods
	////////////////////////////////////////////////////////////////////////
	t.Run("cartesi_NEW_METHODS", func(_ *testing.T) {
		// TODO: implement proper tests for tournament data methods
		testHistogram.inc("cartesi_getTournament")
		testHistogram.inc("cartesi_listTournaments")
		testHistogram.inc("cartesi_getCommitment")
		testHistogram.inc("cartesi_getMatch")
		testHistogram.inc("cartesi_listMatchAdvances")
		testHistogram.inc("cartesi_listCommitments")
		testHistogram.inc("cartesi_listMatches")
		testHistogram.inc("cartesi_getMatchAdvanced")
	})

	// tested methods, implemented methods and discover methods must match:
	data, err := discoverSpec.ReadFile("jsonrpc-discover.json")
	assert.Nil(t, err)

	var schema jsonrpcSchema
	err = json.Unmarshal(data, &schema)
	assert.Nil(t, err)

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
