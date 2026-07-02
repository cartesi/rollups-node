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

	"github.com/cartesi/rollups-node/pkg/ethutil"

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
