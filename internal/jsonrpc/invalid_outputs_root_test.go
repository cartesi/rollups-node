// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/stretchr/testify/require"
)

type invalidOutputsRootAPIRepository struct {
	repository.Repository
	app   *model.Application
	input *model.Input
}

func (r *invalidOutputsRootAPIRepository) GetApplication(context.Context, string) (*model.Application, error) {
	return r.app, nil
}

func (r *invalidOutputsRootAPIRepository) GetInput(context.Context, string, uint64) (*model.Input, error) {
	return r.input, nil
}

func TestInvalidOutputsRootAPIResponses(t *testing.T) {
	reason := "input 7 completed with INVALID_OUTPUTS_ROOT: an accepted yield must declare exactly 32 bytes"
	repo := &invalidOutputsRootAPIRepository{
		app: &model.Application{Name: "invalid-root", Status: model.ApplicationStatus_InvalidOutputsRoot, Reason: &reason},
		input: &model.Input{
			Index: 7, Status: model.InputCompletionStatus_InvalidOutputsRoot,
		},
	}
	s := &Service{repository: repo}
	s.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range []struct {
		method string
		params string
	}{
		{method: "cartesi_getApplication", params: `{"application":"invalid-root"}`},
		{method: "cartesi_getInput", params: `{"application":"invalid-root","input_index":"0x7"}`},
	} {
		t.Run(tc.method, func(t *testing.T) {
			result, err := jsonrpcHandlers[tc.method](s, httptest.NewRequest(http.MethodPost, "/", nil),
				RPCRequest{Params: json.RawMessage(tc.params)})
			require.NoError(t, err)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			var response struct {
				Data struct {
					Status string `json:"status"`
					Reason string `json:"reason"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(encoded, &response))
			require.Equal(t, "INVALID_OUTPUTS_ROOT", response.Data.Status)
			if tc.method == "cartesi_getApplication" {
				require.Equal(t, reason, response.Data.Reason)
			}
		})
	}
}
