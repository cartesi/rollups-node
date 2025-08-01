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

func GetTransactOpts(chainId *big.Int) (*bind.TransactOpts, error) {
	authKind, err := GetAuthKind()
	if err != nil {
		return nil, err
	}
	switch authKind {
	case AuthKindMnemonicVar:
		mnemonic, err := GetAuthMnemonic()
		if err != nil {
			return nil, err
		}
		accountIndex, err := GetAuthMnemonicAccountIndex()
		if err != nil {
			return nil, err
		}
		privateKey, err := ethutil.MnemonicToPrivateKey(mnemonic.Value, accountIndex.Value)
		if err != nil {
			return nil, err
		}
		return bind.NewKeyedTransactorWithChainID(privateKey, chainId)
	case AuthKindPrivateKeyVar:
		privateKey, err := GetAuthPrivateKey()
		if err != nil {
			return nil, err
		}
		key, err := crypto.HexToECDSA(ethutil.TrimHex(privateKey.Value))
		if err != nil {
			return nil, err
		}
		return bind.NewKeyedTransactorWithChainID(key, chainId)
	case AuthKindAWS:
		awsc, err := aws_cfg.LoadDefaultConfig(context.Background())
		if err != nil {
			return nil, err
		}
		kmsConfig := aws_kms.NewFromConfig(awsc)
		authAwsKmsKeyId, err := GetAuthAwsKmsKeyId()
		if err != nil {
			return nil, err
		}
		return signtx.CreateAWSTransactOpts(
			context.Background(),
			kmsConfig,
			aws.String(authAwsKmsKeyId.Value),
			types.NewEIP155Signer(chainId),
		)
	default:
		return nil, fmt.Errorf("no valid authentication method found")
	}
}
