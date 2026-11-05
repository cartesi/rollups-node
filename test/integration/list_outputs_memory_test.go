// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/merkle"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/internal/repository/repotest"
	contractoutputs "github.com/cartesi/rollups-node/pkg/contracts/outputs"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

// TestListOutputsConcurrentResponseBudget reproduces whole-response encoder
// allocation with five independent HTTP requests (not a sequential RPC batch).
// Run against a test-managed node with CARTESI_TEST_NODE_MEMORY_LIMIT_BYTES set
// to constrain its address space; the existing default is 2900000000 bytes.
// Use GOMEMLIMIT=256MiB alongside that virtual-memory cap so Go collects
// temporary objects before its address-space reservations exhaust the cap.
// Before incremental list encoding, these requests can exhaust the API process
// before it can return the response-size error. A transport failure is a test
// failure, not an acceptable substitute for that error.
func TestListOutputsConcurrentResponseBudget(t *testing.T) {
	const count = 10000
	const concurrency = 5
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dsn, err := config.GetDatabaseConnection()
	require.NoError(t, err)
	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	require.NoError(t, err)
	defer repo.Close()
	// Disabled fixture: exercise the real database and API without allowing the
	// reader/advancer/claimer to modify synthetic chain state.
	app := repotest.NewApplicationBuilder().WithName(uniqueAppName("outputs-memory")).WithEnabled(false).Create(ctx, t, repo)
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		require.NoError(t, repo.DeleteApplication(cleanupCtx, app.ID))
	}()
	epoch := repotest.NewEpochBuilder(app.ID).WithIndex(0).WithStatus(model.EpochStatus_Closed).WithBlocks(0, 9).WithInputBounds(0, 1).Build()
	input := repotest.NewInputBuilder().WithIndex(0).WithBlockNumber(5).Build()
	require.NoError(t, repo.CreateEpochsAndInputs(ctx, app.Name, map[*model.Epoch][]*model.Input{epoch: {input}}, 10))
	outputABI, err := contractoutputs.OutputsMetaData.GetAbi()
	require.NoError(t, err)
	raw, err := outputABI.Pack("Notice", []byte("memory-budget"))
	require.NoError(t, err)
	outputs := make([][]byte, count)
	for i := range outputs {
		outputs[i] = raw
	}
	proof := repotest.DummyStateProof()
	require.NoError(t, repo.StoreAdvanceResult(ctx, app.ID, &model.AdvanceResult{
		EpochIndex: 0, InputIndex: 0, Status: model.InputCompletionStatus_Accepted,
		Outputs: outputs, StateProof: *proof,
	}))
	require.NoError(t, repo.UpdateEpochInputsProcessed(ctx, app.Name, 0, proof))
	stored, total, err := repo.ListOutputs(ctx, app.Name, repository.OutputFilter{}, repository.Pagination{Limit: count}, false)
	require.NoError(t, err)
	require.EqualValues(t, count, total)
	require.Len(t, stored, count)
	// Claimed outputs carry a full 63-level Merkle proof, which makes even
	// tiny notices produce a response far larger than the 10 MiB budget.
	for _, output := range stored {
		hash := common.HexToHash("0x01")
		output.Hash = &hash
		output.OutputHashesSiblings = make([]common.Hash, merkle.TREE_DEPTH)
	}
	require.NoError(t, repo.StoreClaimAndProofs(ctx, epoch, stored))
	stored = nil

	client := &http.Client{Timeout: 45 * time.Second}
	defer client.CloseIdleConnections()
	url := envOrDefault("CARTESI_JSONRPC_API_URL", "http://localhost:10011/rpc")
	start := make(chan struct{})
	results := make(chan error, concurrency)
	var ready sync.WaitGroup
	ready.Add(concurrency)
	for i := range concurrency {
		go func(id int) {
			body := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","method":"cartesi_listOutputs","params":{"application":%q,"limit":10000},"id":%d}`, app.Name, id))
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				ready.Done()
				results <- err
				return
			}
			req.Header.Set("Content-Type", "application/json")
			ready.Done()
			<-start
			resp, err := client.Do(req)
			if err != nil {
				results <- err
				return
			}
			defer resp.Body.Close()
			var reply struct {
				ID    int `json:"id"`
				Error *struct {
					Code int `json:"code"`
				} `json:"error"`
			}
			err = json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 4096)).Decode(&reply)
			if err == nil && (resp.StatusCode != http.StatusOK || reply.ID != id || reply.Error == nil || reply.Error.Code != -31003) {
				err = fmt.Errorf("request %d: expected response-size error -31003, got HTTP %d, reply %+v", id, resp.StatusCode, reply)
			}
			results <- err
		}(i)
	}
	ready.Wait()
	close(start)
	for range concurrency {
		require.NoError(t, <-results, "API must reject oversized responses without running out of memory")
	}
	// Confirm the API still serves small reads after the concurrent burst.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(fmt.Sprintf(`{"jsonrpc":"2.0","method":"cartesi_listOutputs","params":{"application":%q,"limit":1},"id":99}`, app.Name)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&reply))
	require.NotEmpty(t, reply.Result)
	require.Empty(t, reply.Error)
}
