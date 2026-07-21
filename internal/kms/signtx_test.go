// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package kms

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/asn1"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/stretchr/testify/require"
)

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

type fakeKMSClient struct {
	t           *testing.T
	privateKey  *ecdsa.PrivateKey
	publicKey   []byte
	signContext context.Context
}

func newFakeKMSClient(t *testing.T, privateKey *ecdsa.PrivateKey) *fakeKMSClient {
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
		SubjectPublicKey: asn1.BitString{Bytes: crypto.FromECDSAPub(&privateKey.PublicKey)},
	})
	require.NoError(t, err)
	return &fakeKMSClient{t: t, privateKey: privateKey, publicKey: publicKey}
}

func (f *fakeKMSClient) GetPublicKey(
	context.Context,
	*awskms.GetPublicKeyInput,
	...func(*awskms.Options),
) (*awskms.GetPublicKeyOutput, error) {
	return &awskms.GetPublicKeyOutput{PublicKey: f.publicKey}, nil
}

func (f *fakeKMSClient) Sign(
	ctx context.Context,
	input *awskms.SignInput,
	_ ...func(*awskms.Options),
) (*awskms.SignOutput, error) {
	f.signContext = ctx
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
