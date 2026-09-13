// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package auth

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	aws_cfg "github.com/aws/aws-sdk-go-v2/config"
	aws_kms "github.com/aws/aws-sdk-go-v2/service/kms"

	. "github.com/cartesi/rollups-node/internal/config"
	signtx "github.com/cartesi/rollups-node/internal/kms"
	"github.com/cartesi/rollups-node/pkg/ethutil"
)

type authGetters struct {
	kind                 func() (AuthKind, error)
	privateKey           func() (RedactedString, error)
	mnemonic             func() (RedactedString, error)
	mnemonicAccountIndex func() (RedactedUint, error)
	awsKMSKeyID          func() (RedactedString, error)
}

func GetTransactOptsFactory(ctx context.Context, chainID *big.Int) (ethutil.TransactOptsFactory, error) {
	return getTransactOptsFactory(ctx, chainID, authGetters{
		kind:                 GetAuthKind,
		privateKey:           GetAuthPrivateKey,
		mnemonic:             GetAuthMnemonic,
		mnemonicAccountIndex: GetAuthMnemonicAccountIndex,
		awsKMSKeyID:          GetAuthAwsKmsKeyId,
	})
}

func GetPrtTransactOptsFactory(ctx context.Context, chainID *big.Int) (ethutil.TransactOptsFactory, error) {
	return getTransactOptsFactory(ctx, chainID, authGetters{
		kind:                 GetPrtAuthKind,
		privateKey:           GetPrtAuthPrivateKey,
		mnemonic:             GetPrtAuthMnemonic,
		mnemonicAccountIndex: GetPrtAuthMnemonicAccountIndex,
		awsKMSKeyID:          GetPrtAuthAwsKmsKeyId,
	})
}

func getTransactOptsFactory(
	ctx context.Context,
	chainID *big.Int,
	getters authGetters,
) (ethutil.TransactOptsFactory, error) {
	if chainID == nil || chainID.Sign() <= 0 {
		return nil, bind.ErrNoChainID
	}

	authKind, err := getters.kind()
	if err != nil {
		return nil, err
	}
	switch authKind {
	case AuthKindMnemonicVar, AuthKindMnemonicFile:
		mnemonic, err := getters.mnemonic()
		if err != nil {
			return nil, err
		}
		accountIndex, err := getters.mnemonicAccountIndex()
		if err != nil {
			return nil, err
		}
		privateKey, err := ethutil.MnemonicToPrivateKey(mnemonic.Value, accountIndex.Value)
		if err != nil {
			return nil, err
		}
		txOpts, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
		if err != nil {
			return nil, err
		}
		return ethutil.NewStaticTransactOptsFactory(txOpts), nil
	case AuthKindPrivateKeyVar, AuthKindPrivateKeyFile:
		privateKey, err := getters.privateKey()
		if err != nil {
			return nil, err
		}
		key, err := crypto.HexToECDSA(ethutil.TrimHex(privateKey.Value))
		if err != nil {
			return nil, err
		}
		txOpts, err := bind.NewKeyedTransactorWithChainID(key, chainID)
		if err != nil {
			return nil, err
		}
		return ethutil.NewStaticTransactOptsFactory(txOpts), nil
	case AuthKindAWS:
		keyID, err := getters.awsKMSKeyID()
		if err != nil {
			return nil, err
		}
		awsCfg, err := aws_cfg.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, err
		}
		kmsClient := aws_kms.NewFromConfig(awsCfg)
		return signtx.CreateAWSTransactOptsFactory(
			ctx,
			kmsClient,
			aws.String(keyID.Value),
			types.LatestSignerForChainID(chainID),
		)
	default:
		return nil, fmt.Errorf("no valid authentication method found")
	}
}
