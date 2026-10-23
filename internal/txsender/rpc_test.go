// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cartesi/rollups-node/internal/errutil"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

const testEstimateGasMethod = "eth_estimateGas"

func TestCapabilityClassificationPreservesPureCancellation(t *testing.T) {
	canceled := fmt.Errorf("account RPC stopped: %w", context.Canceled)
	err := capabilityError(canceled)
	require.True(t, errutil.IsOnlyCancellation(err))
	require.NotErrorIs(t, err, ErrInconclusive)
	joined := errors.Join(canceled, errors.New("unrelated read failure"))
	err = capabilityError(joined)
	require.False(t, errutil.IsOnlyCancellation(err))
	require.ErrorIs(t, err, ErrInconclusive)
	require.ErrorContains(t, err, "unrelated read failure")
}

func TestPreparedNumberEstimateRefusalsRemainVisibleWithoutLatestRetry(t *testing.T) {
	for _, test := range []struct {
		name, message string
		code          int
	}{
		{"unsupported selector", "blockHash not supported for gas estimation", -32602},
		{"execution revert", "execution reverted: custom error 0x0ef47687", 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				var request struct {
					ID     json.RawMessage   `json:"id"`
					Method string            `json:"method"`
					Params []json.RawMessage `json:"params"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				require.Equal(t, testEstimateGasMethod, request.Method)
				require.Len(t, request.Params, 2)
				require.JSONEq(t, `"0x7"`, string(request.Params[1]))
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
					testRPCField: testRPCVersion, "id": request.ID,
					"error": map[string]any{testErrorCodeField: test.code, testErrorMessageField: test.message},
				}))
			}))
			defer server.Close()
			client, err := ethutil.DialEthClient(t.Context(), server.URL)
			require.NoError(t, err)
			defer client.Close()
			backend := ProtectedClient{Client: client}
			_, err = backend.Estimate(t.Context(), ethereum.CallMsg{}, BlockRef{Number: 7, Hash: common.HexToHash("0x1234")})
			require.ErrorContains(t, err, test.message)
			var rpcErr rpc.Error
			require.ErrorAs(t, err, &rpcErr)
			require.Equal(t, test.code, rpcErr.ErrorCode())
			require.Equal(t, int32(1), requests.Load())
		})
	}
}

const (
	testCallMethod        = "eth_call"
	testCodeMethod        = "eth_getCode"
	testRPCVersion        = "2.0"
	testRPCField          = "jsonrpc"
	testResultField       = "result"
	testErrorCodeField    = "code"
	testErrorMessageField = "message"
)

func TestProtectedClientHashReadsUseRequireCanonicalAndExactCallFields(t *testing.T) {
	ref := BlockRef{Number: 7, Hash: common.HexToHash("0x1234")}
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		methods = append(methods, request.Method)
		require.Len(t, request.Params, 2)
		if request.Method == testEstimateGasMethod {
			require.JSONEq(t, `"0x7"`, string(request.Params[1]), "estimation only sizes gas at the prepared height")
		} else {
			var block struct {
				Hash      common.Hash `json:"blockHash"`
				Canonical bool        `json:"requireCanonical"`
			}
			require.NoError(t, json.Unmarshal(request.Params[1], &block))
			require.Equal(t, ref.Hash, block.Hash)
			require.True(t, block.Canonical)
		}
		result := "0x0"
		if request.Method == testEstimateGasMethod {
			result = "0x5208"
		}
		if request.Method == testCodeMethod || request.Method == testCallMethod {
			result = "0x01"
		}
		if request.Method == testCallMethod {
			var fields map[string]string
			require.NoError(t, json.Unmarshal(request.Params[0], &fields))
			require.Equal(t, "0x5208", fields["gas"])
			require.Equal(t, "0x1e", fields["maxFeePerGas"])
			require.Equal(t, "0x2", fields["maxPriorityFeePerGas"])
			require.Equal(t, "0x7", fields["value"])
			require.Equal(t, "0x0102", fields["input"])
			require.NotContains(t, fields, "gasPrice")
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			testRPCField: testRPCVersion, "id": request.ID, testResultField: result,
		}))
	}))
	defer server.Close()
	client, err := ethutil.DialEthClient(t.Context(), server.URL)
	require.NoError(t, err)
	defer client.Close()
	backend := ProtectedClient{Client: client}
	address := common.HexToAddress("0x100")
	_, err = backend.Nonce(t.Context(), address, ref)
	require.NoError(t, err)
	_, err = backend.Balance(t.Context(), address, ref)
	require.NoError(t, err)
	_, err = backend.Code(t.Context(), address, ref)
	require.NoError(t, err)
	_, err = backend.Call(t.Context(), ethereum.CallMsg{
		From: address, To: &address, Gas: 21_000, Value: big.NewInt(7), Data: []byte{1, 2},
		GasFeeCap: big.NewInt(30), GasTipCap: big.NewInt(2),
	}, ref)
	require.NoError(t, err)
	gas, err := backend.Estimate(t.Context(), ethereum.CallMsg{From: address, To: &address}, ref)
	require.NoError(t, err)
	require.Equal(t, uint64(21_000), gas)
	require.Equal(t, []string{"eth_getTransactionCount", "eth_getBalance", testCodeMethod, testCallMethod, testEstimateGasMethod}, methods)
}

func TestProtectedClientCapabilityFailureRequiresSchemaEvidence(t *testing.T) {
	tests := []struct {
		name, message string
		code          int
		unsupported   bool
	}{
		{"missing method", "the method eth_getCode does not exist", -32601, true},
		{"object selector rejected", "object block argument unsupported", -32602, true},
		{"string selector required", "invalid argument 1: json: cannot unmarshal object into Go value of type string", -32602, true},
		{"numeric selector required",
			"invalid argument 1: json: cannot unmarshal object into Go value of type rpc.BlockNumber", -32602, true},
		{"canonical field rejected", "unknown field `requireCanonical`", -32602, true},
		{"hash field rejected", "unknown parameter blockHash", -32602, true},
		{"string schema required", "invalid type: map, expected a string", -32602, true},
		{"historical state unavailable", "BlockOutOfRangeError: block height is 6995 but requested was 6931", -32602, false},
		{"unknown hash", "header not found", -32602, false},
		{"pruned state", "missing trie node", -32000, false},
		{"ambiguous invalid parameters", "invalid params", -32602, false},
		{"execution message cannot assert capability", "execution reverted: requireCanonical unsupported", 3, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cause := testRPCError{code: tt.code, message: tt.message}
			err := capabilityError(fmt.Errorf("account read: %w", cause))
			require.ErrorIs(t, err, cause, "the original provider cause must remain available")
			var rpcErr rpc.Error
			require.ErrorAs(t, err, &rpcErr)
			require.Equal(t, tt.code, rpcErr.ErrorCode())
			if tt.unsupported {
				require.ErrorIs(t, err, ErrUnsupported)
				require.NotErrorIs(t, err, ErrInconclusive)
			} else {
				require.ErrorIs(t, err, ErrInconclusive)
				require.NotErrorIs(t, err, ErrUnsupported)
			}
		})
	}
	require.NoError(t, capabilityError(nil))
	cause := errors.New("account read temporarily unavailable")
	require.ErrorIs(t, capabilityError(cause), cause)
	require.ErrorIs(t, capabilityError(cause), ErrInconclusive)
}

func TestProtectedClientAccountReadsRecoverFromUnavailableHistoricalState(t *testing.T) {
	ref := BlockRef{Number: 6931, Hash: common.HexToHash("0x1234")}
	address := common.HexToAddress("0x100")
	tests := []struct {
		method string
		read   func(ProtectedClient) error
	}{
		{"eth_getTransactionCount", func(c ProtectedClient) error {
			_, err := c.Nonce(t.Context(), address, ref)
			return err
		}},
		{"eth_getBalance", func(c ProtectedClient) error {
			_, err := c.Balance(t.Context(), address, ref)
			return err
		}},
		{testCodeMethod, func(c ProtectedClient) error {
			_, err := c.Code(t.Context(), address, ref)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			var count atomic.Int32
			selectors := make(chan json.RawMessage, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID     json.RawMessage   `json:"id"`
					Method string            `json:"method"`
					Params []json.RawMessage `json:"params"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				require.Equal(t, tt.method, request.Method)
				require.Len(t, request.Params, 2)
				selectors <- request.Params[1]
				response := map[string]any{testRPCField: testRPCVersion, "id": request.ID, testResultField: "0x0"}
				if tt.method == testCodeMethod {
					response[testResultField] = "0x"
				}
				if count.Add(1) == 1 {
					delete(response, testResultField)
					response["error"] = map[string]any{
						"code": -32602, "message": "BlockOutOfRangeError: block height is 6995 but requested was 6931",
					}
				}
				w.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(w).Encode(response))
			}))
			defer server.Close()
			client, err := ethutil.DialEthClient(t.Context(), server.URL)
			require.NoError(t, err)
			defer client.Close()
			backend := ProtectedClient{Client: client}
			err = tt.read(backend)
			require.ErrorIs(t, err, ErrInconclusive)
			require.NotErrorIs(t, err, ErrUnsupported)
			require.ErrorContains(t, err, "BlockOutOfRangeError")
			require.EqualValues(t, 1, count.Load(), "a failed hash read must not fall back to a numeric or latest read")
			require.NoError(t, tt.read(backend), "a recovered historical backend must allow the next protected read")
			require.EqualValues(t, 2, count.Load())
			for range 2 {
				var selector struct {
					Hash      common.Hash `json:"blockHash"`
					Canonical bool        `json:"requireCanonical"`
				}
				require.NoError(t, json.Unmarshal(<-selectors, &selector))
				require.Equal(t, ref.Hash, selector.Hash)
				require.True(t, selector.Canonical)
			}
		})
	}
}

func TestReviewProtectedReadBindingOmitsUnspecifiedGas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, testCallMethod, request.Method)
		require.Len(t, request.Params, 2)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(request.Params[0], &fields))
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": "0x01"}
		// Geth supplies its RPC gas default only for an absent field. An explicit
		// zero is preserved and fails the intrinsic gas check, even for a view.
		if _, gasPresent := fields["gas"]; gasPresent {
			delete(response, "result")
			response["error"] = map[string]any{"code": -32000, "message": "intrinsic gas too low: have 0, want 21000"}
		}
		require.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	defer server.Close()
	client, err := ethutil.DialEthClient(t.Context(), server.URL)
	require.NoError(t, err)
	defer client.Close()
	backend := ProtectedClient{Client: client}
	address := common.HexToAddress("0x100")
	result, err := backend.CallContractAtHash(t.Context(), ethereum.CallMsg{
		From: address, To: &address, Data: []byte{1},
	}, common.HexToHash("0x1234"))
	require.NoError(t, err, "a read binding's unspecified gas must use the protected provider default")
	require.Equal(t, []byte{1}, result)
}
