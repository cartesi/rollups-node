// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package auth

import (
	"context"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestKMSSigningHTTPTimeoutAllowsNextRequest(t *testing.T) {
	normal := newFakeKMSServer(t)
	t.Cleanup(normal.Close)
	var stall atomic.Bool
	stall.Store(true)
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") == "TrentService.Sign" && stall.Swap(false) {
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			started <- struct{}{}
			<-r.Context().Done()
			return
		}
		normal.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	setupAWSAuth(t, server.URL)
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	viper.Set(config.BLOCKCHAIN_HTTP_REQUEST_TIMEOUT, "0.1")
	factory, err := GetTransactOptsFactory(t.Context(), big.NewInt(31337))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	opts, err := factory.NewTransactOpts(ctx)
	require.NoError(t, err)
	to := common.Address{1}
	tx := types.NewTx(&types.LegacyTx{To: &to, Gas: 21000, GasPrice: big.NewInt(1)})
	_, err = opts.Signer(opts.From, tx)
	require.Error(t, err)
	require.NoError(t, ctx.Err(), "the HTTP timeout must bound signing before its caller expires")
	require.Len(t, started, 1, "the server must receive the request before stalling")
	signed, err := opts.Signer(opts.From, tx)
	require.NoError(t, err)
	require.NotNil(t, signed)
}

func TestKMSRequiresPositiveRequestTimeout(t *testing.T) {
	for _, timeout := range []string{"0", "-1"} {
		t.Run(timeout, func(t *testing.T) {
			setupAWSAuth(t, "http://127.0.0.1:1")
			viper.Set(config.BLOCKCHAIN_HTTP_REQUEST_TIMEOUT, timeout)
			_, err := GetTransactOptsFactory(t.Context(), big.NewInt(31337))
			require.ErrorContains(t, err, "must be positive for AWS KMS")
		})
	}
}

func TestKMSRequestTimeoutPreservesCustomCA(t *testing.T) {
	normal := newFakeKMSServer(t)
	t.Cleanup(normal.Close)
	server := httptest.NewTLSServer(normal.Config.Handler)
	t.Cleanup(server.Close)
	setupAWSAuth(t, server.URL)
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	caFile := filepath.Join(t.TempDir(), "kms-ca.pem")
	require.NoError(t, os.WriteFile(caFile,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600))
	t.Setenv("AWS_CA_BUNDLE", caFile)
	viper.Set(config.BLOCKCHAIN_HTTP_REQUEST_TIMEOUT, "1")
	factory, err := GetTransactOptsFactory(t.Context(), big.NewInt(31337))
	require.NoError(t, err, "the AWS SDK must retain custom TLS roots when setting the request timeout")
	require.NotNil(t, factory)
}
