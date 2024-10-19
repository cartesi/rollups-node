package signtx

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
)

var ARN = ""

/* Create a SignTxFn from a private key. Useful for testing */
func CreateSignTxFnFromPrivateKey(privateKey *ecdsa.PrivateKey) SignTxFn {
	return func(tx *types.Transaction, s types.Signer) (*types.Transaction, error) {
		return types.SignTx(tx, s, privateKey)
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
	tx := types.NewTransaction(nonce, recipient, value, gasLimit, gasPrice, data)
	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		panic(err)
	}
	signedTx, err := SignTx(tx, types.NewEIP155Signer(chainID))
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

	anvilPrivateKey, err := ethutil.MnemonicToPrivateKey( ethutil.FoundryMnemonic, 0)
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
