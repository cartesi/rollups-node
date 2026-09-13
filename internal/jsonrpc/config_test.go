// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/evmreader"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/pkg/service"
	"github.com/stretchr/testify/require"
)

type savedConfigRepository struct {
	repository.Repository
	raw []byte
	err error
}

func (r *savedConfigRepository) LoadNodeConfigRaw(
	_ context.Context, key string,
) ([]byte, time.Time, time.Time, error) {
	if key != evmreader.EvmReaderConfigKey {
		panic("unexpected configuration key")
	}
	return r.raw, time.Time{}, time.Time{}, r.err
}

func TestNodeInfoAndChainIDValidateSavedConfig(t *testing.T) {
	const nullConfigJSON = "null"
	for _, method := range []string{"cartesi_getNodeInfo", "cartesi_getChainId"} {
		for _, test := range []struct {
			name     string
			raw      string
			err      error
			wantCode int
		}{
			{"absent", "", repository.ErrNotFound, JSONRPC_RESOURCE_NOT_FOUND},
			{"nil bytes", "", nil, JSONRPC_INTERNAL_ERROR},
			{"null config", nullConfigJSON, nil, JSONRPC_INTERNAL_ERROR},
			{"empty", "{}", nil, JSONRPC_INTERNAL_ERROR},
			{"missing chain", `{"DefaultBlock":"FINALIZED"}`, nil, JSONRPC_INTERNAL_ERROR},
			{"missing policy", `{"ChainID":1}`, nil, JSONRPC_INTERNAL_ERROR},
			{"null chain", `{"ChainID":null,"DefaultBlock":"FINALIZED"}`, nil, JSONRPC_INTERNAL_ERROR},
			{"null policy", `{"ChainID":1,"DefaultBlock":null}`, nil, JSONRPC_INTERNAL_ERROR},
			{"zero chain", `{"ChainID":0,"DefaultBlock":"FINALIZED"}`, nil, JSONRPC_INTERNAL_ERROR},
			{"invalid policy", `{"ChainID":1,"DefaultBlock":"INVALID"}`, nil, JSONRPC_INTERNAL_ERROR},
			{"valid", `{"ChainID":31337,"DefaultBlock":"FINALIZED"}`, nil, 0},
		} {
			t.Run(method+"/"+test.name, func(t *testing.T) {
				var raw []byte
				if test.raw != "" {
					raw = []byte(test.raw)
				}
				s := &Service{
					HTTPServiceTemplate: service.HTTPServiceTemplate{
						BaseTemplate: service.BaseTemplate{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
					},
					repository: &savedConfigRepository{raw: raw, err: test.err},
					handlers:   cloneDispatchTable(jsonrpcHandlers),
				}
				request := httptest.NewRequest("POST", "/rpc", nil)
				result, err := s.handlers[method](s, request, RPCRequest{})
				if test.wantCode != 0 {
					require.Nil(t, result)
					var rpcErr *RPCError
					require.ErrorAs(t, err, &rpcErr)
					require.Equal(t, test.wantCode, rpcErr.Code)
					return
				}
				require.NoError(t, err)
				encoded, err := json.Marshal(result)
				require.NoError(t, err)
				require.Contains(t, string(encoded), "0x7a69")
			})
		}
	}
}
