// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

//go:build endtoendtests

package integration

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/config/auth"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
)

type AwsKmsIntegrationSuite struct {
	suite.Suite
	chainID   *big.Int
	ethClient *ethclient.Client
	kmsClient *awskms.Client
	kmsKeyID  string
	txOpts    *bind.TransactOpts
}

func (s *AwsKmsIntegrationSuite) SetupSuite() {
	t := s.T()
	ctx := t.Context()

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	required := os.Getenv("LOCALSTACK_KMS_REQUIRED") == "true"
	endpoint := os.Getenv("LOCALSTACK_KMS_ENDPOINT")
	if endpoint == "" {
		if required {
			t.Fatal("LOCALSTACK_KMS_ENDPOINT is required for this shard")
		}
		t.Skip("LOCALSTACK_KMS_ENDPOINT is not set; skipping LocalStack KMS integration test")
	}
	cfg, err := awscfg.LoadDefaultConfig(ctx,
		awscfg.WithRegion(region),
		awscfg.WithBaseEndpoint(endpoint),
	)
	s.Require().NoError(err)
	client := awskms.NewFromConfig(cfg)

	created, err := client.CreateKey(ctx, &awskms.CreateKeyInput{
		KeyUsage: kmstypes.KeyUsageTypeSignVerify,
		KeySpec:  kmstypes.KeySpecEccSecgP256k1,
	})
	if err != nil {
		message := "unable to create key on LocalStack"
		if required {
			t.Fatalf("%s: %v", message, err)
		}
		t.Skipf("%s: %v", message, err)
	}
	s.Require().NotNil(created.KeyMetadata)
	s.Require().NotNil(created.KeyMetadata.KeyId)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = client.ScheduleKeyDeletion(cleanupCtx, &awskms.ScheduleKeyDeletionInput{
			KeyId:               created.KeyMetadata.KeyId,
			PendingWindowInDays: aws.Int32(1),
		})
	})

	t.Cleanup(func() {
		viper.Reset()
		viper.AutomaticEnv()
		config.SetDefaults()
	})
	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	s.Require().NoError(err)
	ethClient, err := ethclient.DialContext(ctx, ethEndpoint.Raw())
	s.Require().NoError(err)
	t.Cleanup(ethClient.Close)

	viper.Set(config.AUTH_KIND, "aws")
	viper.Set(config.AUTH_AWS_KMS_KEY_ID, *created.KeyMetadata.KeyId)
	t.Setenv("AWS_REGION", region)
	t.Setenv("AWS_ENDPOINT_URL_KMS", endpoint)

	s.chainID, err = ethClient.ChainID(ctx)
	s.Require().NoError(err)
	factory, err := auth.GetTransactOptsFactory(ctx, s.chainID)
	s.Require().NoError(err)
	s.Require().NotEqual(common.Address{}, factory.From())
	opts, err := factory.NewTransactOpts(ctx)
	s.Require().NoError(err)

	s.ethClient = ethClient
	s.kmsClient = client
	s.kmsKeyID = *created.KeyMetadata.KeyId
	s.txOpts = opts
}

func (s *AwsKmsIntegrationSuite) sendFunds(
	value *big.Int,
	signTx bind.SignerFn,
	sender common.Address,
	recipient common.Address,
) {
	ctx := s.T().Context()

	nonce, err := s.ethClient.PendingNonceAt(ctx, sender)
	s.Require().NoError(err)
	gasLimit := uint64(21000)
	gasTipCap, err := s.ethClient.SuggestGasTipCap(ctx)
	s.Require().NoError(err)
	header, err := s.ethClient.HeaderByNumber(ctx, nil)
	s.Require().NoError(err)
	s.Require().NotNil(header.BaseFee)
	gasFeeCap := new(big.Int).Add(
		new(big.Int).Mul(header.BaseFee, big.NewInt(2)), //nolint:mnd // EIP-1559 base-fee headroom.
		gasTipCap,
	)
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   s.chainID,
		Nonce:     nonce,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Gas:       gasLimit,
		To:        &recipient,
		Value:     value,
	})
	signedTx, err := signTx(sender, tx)
	s.Require().NoError(err)
	err = s.ethClient.SendTransaction(ctx, signedTx)
	s.Require().NoError(err)
	receipt, err := bind.WaitMined(ctx, s.ethClient, signedTx.Hash())
	s.Require().NoError(err)
	s.Require().Equal(types.ReceiptStatusSuccessful, receipt.Status)
}

func (s *AwsKmsIntegrationSuite) TestLocalStackAWSSignedTransaction() {
	// Keep funding transactions isolated from the node submitter (index 0) and
	// the guardian/quorum accounts used by the other integration suites.
	const fundingAccountIndex uint32 = 9
	anvilPrivateKey, err := ethutil.MnemonicToPrivateKey(ethutil.FoundryMnemonic, fundingAccountIndex)
	s.Require().NoError(err)

	anvilPublicKey := anvilPrivateKey.Public().(*ecdsa.PublicKey)
	anvilAddress := crypto.PubkeyToAddress(*anvilPublicKey)
	anvilSignTx := func(address common.Address, tx *types.Transaction) (*types.Transaction, error) {
		if address != anvilAddress {
			return nil, bind.ErrNotAuthorized
		}
		return types.SignTx(tx, types.LatestSignerForChainID(s.chainID), anvilPrivateKey)
	}
	value20 := big.NewInt(2000000000000000000) // in wei (2 eth)
	value10 := big.NewInt(1000000000000000000) // in wei (1 eth)
	s.sendFunds(value20, anvilSignTx, anvilAddress, s.txOpts.From)
	s.sendFunds(value10, s.txOpts.Signer, s.txOpts.From, anvilAddress)
}

func (s *AwsKmsIntegrationSuite) TestLocalStackAWSTransactionOptsFactory() {
	to := common.Address{0x01}
	tests := []struct {
		name string
		tx   *types.Transaction
	}{
		{
			name: "legacy",
			tx: types.NewTx(&types.LegacyTx{
				Nonce: 1, GasPrice: big.NewInt(2), Gas: 21000, To: &to, Value: big.NewInt(3),
			}),
		},
		{
			name: "dynamic fee",
			tx: types.NewTx(&types.DynamicFeeTx{
				ChainID: s.chainID, Nonce: 2, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
				Gas: 21000, To: &to, Value: big.NewInt(3),
			}),
		},
	}
	for _, test := range tests {
		s.Run(test.name, func() {
			signed, err := s.txOpts.Signer(s.txOpts.From, test.tx)
			s.Require().NoError(err)
			sender, err := types.Sender(types.LatestSignerForChainID(s.chainID), signed)
			s.Require().NoError(err)
			s.Require().Equal(s.txOpts.From, sender)
		})
	}
}

func (s *AwsKmsIntegrationSuite) TestWrongKeySpecIsNotTreatedAsTransient() {
	ctx := s.T().Context()
	created, err := s.kmsClient.CreateKey(ctx, &awskms.CreateKeyInput{
		KeyUsage: kmstypes.KeyUsageTypeSignVerify,
		KeySpec:  kmstypes.KeySpecRsa2048,
	})
	s.Require().NoError(err)
	s.Require().NotNil(created.KeyMetadata)
	s.Require().NotNil(created.KeyMetadata.KeyId)
	s.T().Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = s.kmsClient.ScheduleKeyDeletion(cleanupCtx, &awskms.ScheduleKeyDeletionInput{
			KeyId:               created.KeyMetadata.KeyId,
			PendingWindowInDays: aws.Int32(1),
		})
		viper.Set(config.AUTH_AWS_KMS_KEY_ID, s.kmsKeyID)
	})

	viper.Set(config.AUTH_AWS_KMS_KEY_ID, *created.KeyMetadata.KeyId)
	factory, err := auth.GetTransactOptsFactory(ctx, s.chainID)

	s.Require().Nil(factory)
	s.Require().Error(err)
}

func TestLocalStackAWSIntegration(t *testing.T) {
	suite.Run(t, new(AwsKmsIntegrationSuite))
}
