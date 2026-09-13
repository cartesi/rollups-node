// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package auth

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	. "github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

func TestGetTransactOptsFactoryAcceptsVariableAndFileKinds(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	privateKeyText := hex.EncodeToString(crypto.FromECDSA(privateKey))
	privateKeyAddress := crypto.PubkeyToAddress(privateKey.PublicKey)

	mnemonicKey, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, 4)
	require.NoError(t, err)
	mnemonicAddress := crypto.PubkeyToAddress(mnemonicKey.PublicKey)

	tests := []struct {
		name            string
		kind            AuthKind
		expectedAddress common.Address
	}{
		{name: "private key variable", kind: AuthKindPrivateKeyVar, expectedAddress: privateKeyAddress},
		{name: "private key file", kind: AuthKindPrivateKeyFile, expectedAddress: privateKeyAddress},
		{name: "mnemonic variable", kind: AuthKindMnemonicVar, expectedAddress: mnemonicAddress},
		{name: "mnemonic file", kind: AuthKindMnemonicFile, expectedAddress: mnemonicAddress},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory, err := getTransactOptsFactory(t.Context(), big.NewInt(31337), authGetters{
				kind: func() (AuthKind, error) { return test.kind, nil },
				privateKey: func() (RedactedString, error) {
					return RedactedString{Value: privateKeyText}, nil
				},
				mnemonic: func() (RedactedString, error) {
					return RedactedString{Value: ethutil.FoundryMnemonic}, nil
				},
				mnemonicAccountIndex: func() (RedactedUint, error) {
					return RedactedUint{Value: 4}, nil
				},
				awsKMSKeyID: func() (RedactedString, error) {
					t.Fatal("AWS KMS getter must not be called")
					return RedactedString{}, nil
				},
			})
			require.NoError(t, err)
			require.Equal(t, test.expectedAddress, factory.From())
		})
	}
}

func TestGetPrtTransactOptsFactoryUsesOnlyPrtAuth(t *testing.T) {
	for _, test := range []struct {
		name  string
		index string
		want  uint32
	}{
		{name: "default", want: 6},
		{name: "explicit nonzero index", index: "4", want: 4},
		{name: "explicit zero index", index: "0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetAuthConfig(t)
			t.Setenv(PRT_AUTH_MNEMONIC_ACCOUNT_INDEX, test.index)
			viper.Set(AUTH_KIND, "mnemonic")
			viper.Set(AUTH_MNEMONIC, ethutil.FoundryMnemonic)
			viper.Set(AUTH_MNEMONIC_ACCOUNT_INDEX, 0)
			viper.Set(PRT_AUTH_KIND, "mnemonic")
			viper.Set(PRT_AUTH_MNEMONIC, ethutil.FoundryMnemonic)

			claimerFactory, err := GetTransactOptsFactory(t.Context(), big.NewInt(31337))
			require.NoError(t, err)
			prtFactory, err := GetPrtTransactOptsFactory(t.Context(), big.NewInt(31337))
			require.NoError(t, err)
			prtKey, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, test.want)
			require.NoError(t, err)
			require.Equal(t, crypto.PubkeyToAddress(prtKey.PublicKey), prtFactory.From())
			if test.want != 0 {
				require.NotEqual(t, claimerFactory.From(), prtFactory.From())
			}
		})
	}
}

func TestGetPrtTransactOptsFactoryDoesNotFallbackToGenericAuth(t *testing.T) {
	resetAuthConfig(t)
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	viper.Set(AUTH_KIND, "private_key")
	viper.Set(AUTH_PRIVATE_KEY, hex.EncodeToString(crypto.FromECDSA(privateKey)))
	viper.Set(PRT_AUTH_KIND, "private_key")

	factory, err := GetPrtTransactOptsFactory(t.Context(), big.NewInt(31337))
	require.Nil(t, factory)
	require.ErrorContains(t, err, PRT_AUTH_PRIVATE_KEY)
}

func TestGetPrtTransactOptsFactoryRequiresPrtMnemonic(t *testing.T) {
	resetAuthConfig(t)
	viper.Set(AUTH_KIND, "mnemonic")
	viper.Set(AUTH_MNEMONIC, ethutil.FoundryMnemonic)
	viper.Set(PRT_AUTH_KIND, "mnemonic")
	viper.Set(PRT_AUTH_MNEMONIC, "")

	factory, err := GetPrtTransactOptsFactory(t.Context(), big.NewInt(31337))
	require.Nil(t, factory)
	require.ErrorContains(t, err, PRT_AUTH_MNEMONIC)
}

func TestGetPrtTransactOptsFactoryReadsFileAuth(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	privateKeyText := hex.EncodeToString(crypto.FromECDSA(privateKey))
	privateKeyFile := filepath.Join(t.TempDir(), "prt-private-key")
	require.NoError(t, os.WriteFile(privateKeyFile, []byte(privateKeyText), 0o600))

	resetAuthConfig(t)
	viper.Set(PRT_AUTH_KIND, "private_key_file")
	viper.Set(PRT_AUTH_PRIVATE_KEY_FILE, privateKeyFile)

	factory, err := GetPrtTransactOptsFactory(t.Context(), big.NewInt(31337))
	require.NoError(t, err)
	require.Equal(t, crypto.PubkeyToAddress(privateKey.PublicKey), factory.From())
}

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

func TestGetTransactOptsFactoryRejectsInvalidChainID(t *testing.T) {
	tests := []struct {
		name    string
		chainID *big.Int
	}{
		{name: "nil", chainID: nil},
		{name: "zero", chainID: big.NewInt(0)},
		{name: "negative", chainID: big.NewInt(-1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory, err := GetTransactOptsFactory(t.Context(), test.chainID)
			require.Nil(t, factory)
			require.ErrorIs(t, err, bind.ErrNoChainID)
		})
	}
}

func resetAuthConfig(t *testing.T) {
	t.Helper()
	viper.Reset()
	viper.AutomaticEnv()
	SetDefaults()
	t.Cleanup(func() {
		viper.Reset()
		viper.AutomaticEnv()
		SetDefaults()
	})
}

func setupAWSAuth(t *testing.T, endpoint string) {
	t.Helper()
	resetAuthConfig(t)
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
