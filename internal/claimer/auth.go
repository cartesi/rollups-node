// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package claimer

import (
	"context"
	"fmt"

	"github.com/cartesi/rollups-node/internal/config"
	signtx "github.com/cartesi/rollups-node/internal/kms"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	aws_cfg "github.com/aws/aws-sdk-go-v2/config"
	aws_kms "github.com/aws/aws-sdk-go-v2/service/kms"
)

var (
	ENoAuth = fmt.Errorf("error: unimplemented authentication method")
)

func CreateSignerFromAuth(
	auth config.Auth,
	ctx context.Context,
	client *ethclient.Client,
) (
	*bind.TransactOpts,
	error,
) {
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return nil, err
	}

	switch auth := auth.(type) {
	case config.AuthPrivateKey:
		key, err := crypto.HexToECDSA(auth.PrivateKey.Value)
		if err != nil {
			return nil, err
		}
		return bind.NewKeyedTransactorWithChainID(key, chainID)
	case config.AuthMnemonic:
		privateKey, err := ethutil.MnemonicToPrivateKey(
			auth.Mnemonic.Value, uint32(auth.AccountIndex.Value))
		if err != nil {
			return nil, err
		}
		return bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	case config.AuthAWS:
		awsc, err := aws_cfg.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, err
		}
		kms := aws_kms.NewFromConfig(awsc)
		// TODO: option for an alternative endpoint
		//kms := aws_kms.NewFromConfig(awsc, func(o *aws_kms.Options) {
		//	o.BaseEndpoint = aws.String(auth.EndpointURL.Value)
		//})
		return signtx.CreateAWSTransactOpts(ctx, kms,
			aws.String(auth.KeyID.Value), types.NewEIP155Signer(chainID))
	}
	return nil, ENoAuth
}
