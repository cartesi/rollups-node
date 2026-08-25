// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package auth

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	. "github.com/cartesi/rollups-node/internal/config"
)

func TestGetTransactOptsFactoryAWSSignsDynamicFeeTransaction(t *testing.T) {
	server := newFakeKMSServer(t)
	t.Cleanup(server.Close)
	setupAWSAuth(t, server.URL)

	chainID := big.NewInt(31337)
	factory, err := GetTransactOptsFactory(t.Context(), chainID)
	require.NoError(t, err)
	opts, err := factory.NewTransactOpts(t.Context())
	require.NoError(t, err)

	to := common.Address{0x01}
	tests := []struct {
		name string
		tx   *types.Transaction
	}{
		{
			name: "dynamic fee",
			tx: types.NewTx(&types.DynamicFeeTx{
				ChainID:   chainID,
				Nonce:     1,
				GasTipCap: big.NewInt(1),
				GasFeeCap: big.NewInt(2),
				Gas:       21000,
				To:        &to,
				Value:     big.NewInt(3),
			}),
		},
		{
			name: "legacy",
			tx: types.NewTx(&types.LegacyTx{
				Nonce:    2,
				GasPrice: big.NewInt(1),
				Gas:      21000,
				To:       &to,
				Value:    big.NewInt(3),
			}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			signed, err := opts.Signer(opts.From, test.tx)
			require.NoError(t, err)
			sender, err := types.Sender(types.LatestSignerForChainID(chainID), signed)
			require.NoError(t, err)
			require.Equal(t, opts.From, sender)
		})
	}
}

func setupAWSAuth(t *testing.T, endpoint string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set(AUTH_KIND, "aws")
	viper.Set(AUTH_AWS_KMS_KEY_ID, "alias/test-key")

	// Static dummy credentials keep the AWS SDK hermetic: it never consults
	// shared config files, credential services, or EC2 instance metadata.
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ENDPOINT_URL_KMS", endpoint)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

func newFakeKMSServer(t *testing.T) *httptest.Server {
	t.Helper()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	publicKey, err := asn1.Marshal(struct {
		Algorithm struct {
			Algorithm  asn1.ObjectIdentifier
			Parameters asn1.ObjectIdentifier
		}
		SubjectPublicKey asn1.BitString
	}{
		Algorithm: struct {
			Algorithm  asn1.ObjectIdentifier
			Parameters asn1.ObjectIdentifier
		}{
			Algorithm:  asn1.ObjectIdentifier{1, 2, 840, 10045, 2, 1},
			Parameters: asn1.ObjectIdentifier{1, 3, 132, 0, 10},
		},
		SubjectPublicKey: asn1.BitString{Bytes: crypto.FromECDSAPub(&privateKey.PublicKey)},
	})
	require.NoError(t, err)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		switch r.Header.Get("X-Amz-Target") {
		case "TrentService.GetPublicKey":
			writeKMSJSON(t, w, map[string]any{
				"KeyId":     "alias/test-key",
				"KeySpec":   "ECC_SECG_P256K1",
				"KeyUsage":  "SIGN_VERIFY",
				"PublicKey": base64.StdEncoding.EncodeToString(publicKey),
			})
		case "TrentService.Sign":
			var input struct {
				Message string `json:"Message"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
			digest, err := base64.StdEncoding.DecodeString(input.Message)
			require.NoError(t, err)
			r, s, err := ecdsa.Sign(rand.Reader, privateKey, digest)
			require.NoError(t, err)
			signature, err := asn1.Marshal(struct {
				R *big.Int
				S *big.Int
			}{R: r, S: s})
			require.NoError(t, err)
			writeKMSJSON(t, w, map[string]any{
				"KeyId":            "alias/test-key",
				"Signature":        base64.StdEncoding.EncodeToString(signature),
				"SigningAlgorithm": "ECDSA_SHA_256",
			})
		default:
			http.Error(w, "unexpected KMS operation", http.StatusBadRequest)
		}
	}))
}

func writeKMSJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	require.NoError(t, json.NewEncoder(w).Encode(value))
}
