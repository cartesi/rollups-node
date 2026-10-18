// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package txsender

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

type wirePublicationBackend struct {
	*fakeBackend
	publication ProtectedClient
}

func (b wirePublicationBackend) Publish(ctx context.Context, raw []byte) (common.Hash, error) {
	return b.publication.Publish(ctx, raw)
}

func TestLostPublicationReplyRetriesIdenticalBytesAndReturnsKnownWithoutError(t *testing.T) {
	var mu sync.Mutex
	var posted [][]byte
	var handlerErrors []error
	recordError := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		handlerErrors = append(handlerErrors, err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params []hexutil.Bytes `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			recordError(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if request.Method != "eth_sendRawTransaction" || len(request.Params) != 1 {
			recordError(fmt.Errorf("unexpected publication request %q", request.Method))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		posted = append(posted, append([]byte(nil), request.Params[0]...))
		first := len(posted) == 1
		mu.Unlock()
		if first {
			// The provider records the bytes, then loses its response. The
			// production retry transport must post the same bytes again.
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				recordError(err)
				return
			}
			if err := conn.Close(); err != nil {
				recordError(err)
			}
			return
		}
		response := struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Error   struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}{JSONRPC: "2.0", ID: request.ID}
		response.Error.Code, response.Error.Message = -32000, knownPublicationMessage
		if err := json.NewEncoder(w).Encode(response); err != nil {
			recordError(err)
		}
	}))
	defer server.Close()
	client, err := ethutil.NewEthClient(t.Context(), server.URL, testLogger(), ethutil.RetryConfig{
		MaxRetries: 2, RetryMinWait: time.Millisecond, RetryMaxWait: time.Millisecond, RequestTimeout: time.Second,
	})
	require.NoError(t, err)
	defer client.Close()
	s, backend, signer := testSender(t)
	s.backend = wirePublicationBackend{fakeBackend: backend, publication: ProtectedClient{Client: client}}
	planner := newPlanner()
	for block := uint64(1); block <= 2; block++ {
		backend.advance(block, 0)
		o, err := s.PublishNext(t.Context(), planner)
		require.NoError(t, err)
		require.Equal(t, Known, o.Class)
		require.True(t, o.Attempted)
	}
	mu.Lock()
	defer mu.Unlock()
	require.Empty(t, handlerErrors)
	require.Len(t, posted, 3, "one HTTP retry, then one rebroadcast on the following block")
	require.Equal(t, posted[0], posted[1])
	require.Equal(t, posted[0], posted[2])
	require.Equal(t, 1, signer.signs)
}

func TestProtectedPreparationThroughRetryClientReturnsTransportFailure(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusBadGateway} {
		for _, retries := range []uint64{0, 2} {
			t.Run(fmt.Sprintf("http%d/retries%d", status, retries), func(t *testing.T) {
				var calls atomic.Uint64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					calls.Add(1)
					w.WriteHeader(status)
				}))
				defer server.Close()
				client, err := ethutil.NewEthClient(t.Context(), server.URL, testLogger(), ethutil.RetryConfig{
					MaxRetries: retries, RetryMinWait: time.Millisecond, RetryMaxWait: time.Millisecond, RequestTimeout: time.Second,
				})
				require.NoError(t, err)
				defer client.Close()
				backend := ProtectedClient{Client: client}
				to := common.HexToAddress("0x100")
				_, err = backend.Call(t.Context(), ethereum.CallMsg{To: &to, Data: []byte{1}},
					BlockRef{Hash: common.HexToHash("0x1")})
				require.Error(t, err)
				var transport *url.Error
				require.ErrorAs(t, err, &transport)
				var rpcHTTP rpc.HTTPError
				require.False(t, errors.As(err, &rpcHTTP), "production retry exhaustion is not a raw RPC HTTP error")
				require.Equal(t, retries+1, calls.Load())
				require.Equal(t, Uncertain, ClassifyPublication(err), "transport failures cannot create fee evidence")
			})
		}
	}
}
