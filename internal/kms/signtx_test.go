// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package kms

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/asn1"
	"errors"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/stretchr/testify/require"
)

var ARN = ""

/* Create a SignTxFn from a private key. Useful for testing */
func CreateSignTxFnFromPrivateKey(privateKey *ecdsa.PrivateKey) SignTxFn {
	return func(_ context.Context, tx *ethtypes.Transaction, s ethtypes.Signer) (*ethtypes.Transaction, error) {
		return ethtypes.SignTx(tx, s, privateKey)
	}
}

func TestAssembleSignatureRejectsOverlongComponents(t *testing.T) {
	tests := []struct {
		name     string
		r        []byte
		s        []byte
		expected string
	}{
		{
			name: "r", r: make([]byte, 33), s: make([]byte, 32),
			expected: "malformed signature: len(r)=33 len(s)=32",
		},
		{
			name: "s", r: make([]byte, 32), s: make([]byte, 33),
			expected: "malformed signature: len(r)=32 len(s)=33",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			signature, err := assembleSignature(test.r, test.s, nil, nil)
			require.Nil(t, signature)
			require.EqualError(t, err, test.expected)
		})
	}
}

func TestNormalizeR(t *testing.T) {
	t.Run("keeps components up to 32 bytes", func(t *testing.T) {
		input := []byte{1, 2, 3}

		r, err := normalizeR(input)
		require.NoError(t, err)
		require.Equal(t, input, r)
	})

	t.Run("trims leading zero padding", func(t *testing.T) {
		padded := append([]byte{0}, make([]byte, 32)...)
		padded[len(padded)-1] = 1

		r, err := normalizeR(padded)
		require.NoError(t, err)
		require.Len(t, r, 32)
		require.Equal(t, byte(1), r[len(r)-1])
	})

	t.Run("rejects non-padding bytes", func(t *testing.T) {
		malformed := append([]byte{1}, make([]byte, 32)...)

		r, err := normalizeR(malformed)
		require.Nil(t, r)
		require.EqualError(t, err, "malformed `r` component")
	})
}

func TestNormalizeSConvertsHighSToLowS(t *testing.T) {
	n := crypto.S256().Params().N
	halfN := new(big.Int).Div(new(big.Int).Set(n), big.NewInt(2)) //nolint:mnd
	highS := new(big.Int).Add(halfN, big.NewInt(1))
	expected := new(big.Int).Sub(n, highS).Bytes()

	require.Equal(t, expected, normalizeS(highS.Bytes()))
}

func TestAssembleSignatureRejectsUnrecoverableKey(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	otherKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	hash := crypto.Keccak256([]byte("test transaction"))
	signature, err := crypto.Sign(hash, privateKey)
	require.NoError(t, err)

	assembled, err := assembleSignature(
		signature[:32], signature[32:64], hash, crypto.FromECDSAPub(&otherKey.PublicKey),
	)
	require.EqualError(t, err, "failed to compute signature")
	require.NotNil(t, assembled)
}

func sendFunds(
	value *big.Int,
	SignTx SignTxFn,
	ctx context.Context,
	sender common.Address,
	recipient common.Address,
) {
	client, err := ethclient.Dial("http://127.0.0.1:8545") // anvil
	if err != nil {
		panic(err)
	}

	nonce, err := client.PendingNonceAt(context.Background(), sender)
	if err != nil {
		panic(err)
	}
	gasLimit := uint64(21000)
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		panic(err)
	}
	var data []byte
	tx := ethtypes.NewTransaction(nonce, recipient, value, gasLimit, gasPrice, data)
	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		panic(err)
	}
	signedTx, err := SignTx(ctx, tx, ethtypes.NewEIP155Signer(chainID))
	if err != nil {
		panic(err)
	}
	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		panic(err)
	}
}

func TestSignTx(t *testing.T) {
	if len(ARN) == 0 {
		t.Skip("Skipping test, ARN for KMS key is unset")
	}
	value20 := big.NewInt(2000000000000000000) // in wei (2 eth)
	value10 := big.NewInt(1000000000000000000) // in wei (1 eth)

	anvilPrivateKey, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, 0)
	if err != nil {
		panic(err)
	}
	anvilPublicKey := anvilPrivateKey.Public().(*ecdsa.PublicKey)
	anvilAddress := crypto.PubkeyToAddress(*anvilPublicKey)

	config, err := awscfg.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(err)
	}
	kms := awskms.NewFromConfig(config)
	SignTx, _, KMSAddress, err := CreateAWSSignTxFn(context.Background(), kms, &ARN)
	if err != nil {
		panic(err)
	}

	sendFunds(value20, CreateSignTxFnFromPrivateKey(anvilPrivateKey),
		context.Background(), anvilAddress, KMSAddress)
	sendFunds(value10, SignTx,
		context.Background(), KMSAddress, anvilAddress)
}

func TestAWSTransactOptsFactorySignsWithSubmitContext(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	client := newFakeKMSClient(t, privateKey)
	arn := "alias/test-key"
	startupCtx, cancelStartup := context.WithCancel(context.Background())
	factory, err := CreateAWSTransactOptsFactory(
		startupCtx,
		client,
		&arn,
		ethtypes.NewEIP155Signer(big.NewInt(1)),
	)
	require.NoError(t, err)
	cancelStartup()
	require.Equal(t, crypto.PubkeyToAddress(privateKey.PublicKey), factory.From())

	type contextKey string
	submitCtx := context.WithValue(context.Background(), contextKey("phase"), "submit")
	opts, err := factory.NewTransactOpts(submitCtx)
	require.NoError(t, err)

	tx := ethtypes.NewTransaction(0, common.Address{0x01}, big.NewInt(1), 21000, big.NewInt(1), nil)
	_, err = opts.Signer(opts.From, tx)
	require.NoError(t, err)
	require.Equal(t, "submit", client.signContext.Value(contextKey("phase")))
	require.NoError(t, client.signContext.Err())
}

func TestAWSTransactOptsFactorySignsDynamicFeeTransaction(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)

	chainID := big.NewInt(31337)
	client := newFakeKMSClient(t, privateKey)
	keyID := "alias/test-key"
	factory, err := CreateAWSTransactOptsFactory(
		context.Background(), client, &keyID, ethtypes.LatestSignerForChainID(chainID),
	)
	require.NoError(t, err)

	opts, err := factory.NewTransactOpts(context.Background())
	require.NoError(t, err)
	tx := ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     1,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(2),
		Gas:       21000,
		To:        &common.Address{0x01},
		Value:     big.NewInt(3),
	})
	signed, err := opts.Signer(opts.From, tx)
	require.NoError(t, err)

	sender, err := ethtypes.Sender(ethtypes.LatestSignerForChainID(chainID), signed)
	require.NoError(t, err)
	require.Equal(t, crypto.PubkeyToAddress(privateKey.PublicKey), sender)
}

func TestAWSTransactOptsFactoryRejectsUnauthorizedAddress(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	client := newFakeKMSClient(t, privateKey)
	arn := "alias/test-key"
	factory, err := CreateAWSTransactOptsFactory(
		context.Background(), client, &arn, ethtypes.NewEIP155Signer(big.NewInt(1)),
	)
	require.NoError(t, err)
	opts, err := factory.NewTransactOpts(context.Background())
	require.NoError(t, err)
	tx := ethtypes.NewTransaction(0, common.Address{0x01}, big.NewInt(1), 21000, big.NewInt(1), nil)

	signed, err := opts.Signer(common.Address{0xff}, tx)
	require.Nil(t, signed)
	require.ErrorIs(t, err, bind.ErrNotAuthorized)
	require.Zero(t, client.signCalls)
}

func TestAWSSignTxRejectsNonCanonicalDERComponents(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	arn := "alias/test-key"
	tx := ethtypes.NewTransaction(0, common.Address{0x01}, big.NewInt(1), 21000, big.NewInt(1), nil)
	signer := ethtypes.NewEIP155Signer(big.NewInt(1))

	tests := []struct {
		name     string
		r        []byte
		s        []byte
		expected string
	}{
		{
			name: "non-padding byte in overlong r", r: append([]byte{1}, make([]byte, 32)...), s: []byte{1},
			expected: "malformed `r` component",
		},
		{
			name: "non-minimal overlong s", r: []byte{1}, s: append([]byte{0}, make([]byte, 32)...),
			expected: "malformed signature: len(r)=1 len(s)=33",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newFakeKMSClient(t, privateKey)
			client.signature = marshalRawECDSASignature(test.r, test.s)
			signTx, _, _, err := CreateAWSSignTxFn(context.Background(), client, &arn)
			require.NoError(t, err)

			signed, err := signTx(context.Background(), tx, signer)
			require.Nil(t, signed)
			require.EqualError(t, err, test.expected)
		})
	}
}

func TestAWSSignTxPropagatesKMSSignFailure(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	client := newFakeKMSClient(t, privateKey)
	signErr := errors.New("KMSInternalException")
	client.signErr = signErr
	keyID := testKeyID
	signTx, _, _, err := CreateAWSSignTxFn(t.Context(), client, &keyID)
	require.NoError(t, err)

	chainID := big.NewInt(31337)
	tx := ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID: chainID, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2), Gas: 21000,
	})
	signed, err := signTx(t.Context(), tx, ethtypes.LatestSignerForChainID(chainID))

	require.Nil(t, signed)
	require.ErrorIs(t, err, signErr)
}

func TestAWSSignTxRejectsMalformedKMSSignature(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	keyID := testKeyID
	tx := ethtypes.NewTransaction(0, common.Address{0x01}, big.NewInt(1), 21000, big.NewInt(1), nil)
	signer := ethtypes.LatestSignerForChainID(big.NewInt(1))

	for _, test := range []struct {
		name      string
		signature []byte
	}{
		{name: "not DER", signature: []byte{0xff, 0xff, 0xff}},
		{name: "truncated sequence", signature: []byte{0x30, 0x03, 0x02, 0x01, 0x01}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newFakeKMSClient(t, privateKey)
			client.signature = test.signature
			signTx, _, _, err := CreateAWSSignTxFn(t.Context(), client, &keyID)
			require.NoError(t, err)

			signed, err := signTx(t.Context(), tx, signer)
			require.Nil(t, signed)
			require.Error(t, err)
		})
	}
}

func TestCreateAWSTransactOptsFactoryPropagatesGetPublicKeyFailure(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	client := newFakeKMSClient(t, privateKey)
	getPublicKeyErr := errors.New("KeyUnavailableException")
	client.getPublicKeyErr = getPublicKeyErr
	keyID := testKeyID

	factory, err := CreateAWSTransactOptsFactory(
		t.Context(), client, &keyID, ethtypes.LatestSignerForChainID(big.NewInt(1)),
	)

	require.Nil(t, factory)
	require.ErrorIs(t, err, getPublicKeyErr)
}

func TestCreateAWSTransactOptsFactoryRejectsMalformedPublicKeys(t *testing.T) {
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	keyID := testKeyID
	tests := []struct {
		name      string
		publicKey []byte
	}{
		{name: "malformed DER", publicKey: []byte{0xff, 0xff}},
		{name: "invalid secp256k1 point", publicKey: marshalSPKI(t, []byte{0x04, 0x01, 0x02})},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newFakeKMSClient(t, privateKey)
			client.publicKey = test.publicKey
			factory, err := CreateAWSTransactOptsFactory(
				t.Context(), client, &keyID, ethtypes.LatestSignerForChainID(big.NewInt(1)),
			)
			require.Nil(t, factory)
			require.Error(t, err)
		})
	}
}

func marshalRawECDSASignature(r, s []byte) []byte {
	content := make([]byte, 0, len(r)+len(s)+4)
	content = append(content, 0x02, byte(len(r)))
	content = append(content, r...)
	content = append(content, 0x02, byte(len(s)))
	content = append(content, s...)
	return append([]byte{0x30, byte(len(content))}, content...)
}

type fakeKMSClient struct {
	t               *testing.T
	privateKey      *ecdsa.PrivateKey
	publicKey       []byte
	signContext     context.Context
	signCalls       int
	signature       []byte
	getPublicKeyErr error
	signErr         error
}

func newFakeKMSClient(t *testing.T, privateKey *ecdsa.PrivateKey) *fakeKMSClient {
	t.Helper()
	return &fakeKMSClient{
		t: t, privateKey: privateKey, publicKey: marshalSPKI(t, crypto.FromECDSAPub(&privateKey.PublicKey)),
	}
}

func marshalSPKI(t *testing.T, publicKeyBytes []byte) []byte {
	t.Helper()
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
		SubjectPublicKey: asn1.BitString{Bytes: publicKeyBytes},
	})
	require.NoError(t, err)
	return publicKey
}

func (f *fakeKMSClient) GetPublicKey(
	context.Context,
	*awskms.GetPublicKeyInput,
	...func(*awskms.Options),
) (*awskms.GetPublicKeyOutput, error) {
	if f.getPublicKeyErr != nil {
		return nil, f.getPublicKeyErr
	}
	return &awskms.GetPublicKeyOutput{PublicKey: f.publicKey}, nil
}

func (f *fakeKMSClient) Sign(
	ctx context.Context,
	input *awskms.SignInput,
	_ ...func(*awskms.Options),
) (*awskms.SignOutput, error) {
	f.signCalls++
	f.signContext = ctx
	if f.signErr != nil {
		return nil, f.signErr
	}
	if f.signature != nil {
		return &awskms.SignOutput{Signature: f.signature}, nil
	}
	r, s, err := ecdsa.Sign(rand.Reader, f.privateKey, input.Message)
	require.NoError(f.t, err)
	signature, err := asn1.Marshal(struct {
		R *big.Int
		S *big.Int
	}{R: r, S: s})
	require.NoError(f.t, err)
	return &awskms.SignOutput{
		KeyId:            input.KeyId,
		SigningAlgorithm: kmstypes.SigningAlgorithmSpecEcdsaSha256,
		Signature:        signature,
	}, nil
}
