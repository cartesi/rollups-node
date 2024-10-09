// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"log/slog"
	"math/big"
	"os"
	"strings"

	cmdcommom "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/machine"
	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iauthorityfactory"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "deploy",
	Short:   "Deploy an application and add it to the node",
	Example: examples,
	Run:     run,
}

const examples = `# Adds an application to Rollups Node:
cartesi-rollups-cli app deploy -a 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF -i 0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA -t applications/echo-dapp` //nolint:lll

const (
	statusRunning    = "running"
	statusNotRunning = "not-running"
)

var (
	owner                string
	templatePath         string
	status               string
	iConsensusAddr       string
	appFactoryAddr       string
	authorityFactoryAddr string
	rpcURL               string
	privateKey           string
	mnemonic             string
	salt                 string
	inputBoxBlockNumber  uint64
	epochLength          uint64
)

func init() {
	Cmd.Flags().StringVarP(
		&owner,
		"owner",
		"o",
		"0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
		"Application owner",
	)

	Cmd.Flags().StringVarP(
		&templatePath,
		"template-path",
		"t",
		"",
		"Application template URI",
	)
	cobra.CheckErr(Cmd.MarkFlagRequired("template-path"))

	Cmd.Flags().StringVarP(
		&status,
		"status",
		"s",
		statusRunning,
		"Sets the application status",
	)

	Cmd.Flags().StringVarP(
		&appFactoryAddr,
		"app-factory",
		"a",
		"0xA1DA32BF664109D62208a1cb0d69aACc6a484873",
		"Application Factory Address",
	)

	Cmd.Flags().StringVarP(
		&authorityFactoryAddr,
		"authority-factory",
		"c",
		"0xbDC5D42771A4Ae55eC7670AAdD2458D1d9C7C8A8",
		"Authority Factory Address",
	)

	Cmd.Flags().StringVarP(
		&iConsensusAddr,
		"iconsensus",
		"i",
		"",
		"Application IConsensus Address",
	)

	Cmd.Flags().StringVar(&rpcURL, "rpc-url", "http://localhost:8545", "Ethereum RPC URL")
	Cmd.Flags().StringVar(&privateKey, "private-key", "", "Private key for signing transactions")
	Cmd.Flags().StringVar(&mnemonic, "mnemonic", ethutil.FoundryMnemonic, "Mnemonic for signing transactions")
	Cmd.Flags().StringVar(&salt, "salt", "0000000000000000000000000000000000000000000000000000000000000000", "salt")
	Cmd.Flags().Uint64VarP(&inputBoxBlockNumber, "inputbox-block-number", "n", 0, "InputBox deployment block number")
	Cmd.Flags().Uint64VarP(&epochLength, "epoch-length", "e", 10, "Consensus Epoch length")
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	if cmdcommom.Database == nil {
		panic("Database was not initialized")
	}

	var applicationStatus model.ApplicationStatus
	switch status {
	case statusRunning:
		applicationStatus = model.ApplicationStatusRunning
	case statusNotRunning:
		applicationStatus = model.ApplicationStatusNotRunning
	default:
		slog.Error("Invalid application status", "status", status)
		os.Exit(1)
	}

	templateHash, err := machine.ReadHash(templatePath)
	if err != nil {
		slog.Error("Read machine template hash failed", "error", err)
		os.Exit(1)
	}

	authorityFactoryAddress := common.HexToAddress(authorityFactoryAddr)
	authorityAddr, err := deployAuthority(ctx, owner, authorityFactoryAddress, epochLength, salt)
	if err != nil {
		slog.Error("Authoriy contract creation failed", "error", err)
		os.Exit(1)
	}

	applicationFactoryAddress := common.HexToAddress(appFactoryAddr)
	appAddr, err := deployApplication(ctx, owner, applicationFactoryAddress, authorityAddr, templateHash, salt)
	if err != nil {
		slog.Error("Application contract creation failed", "error", err)
		os.Exit(1)
	}

	application := model.Application{
		ContractAddress:    appAddr,
		TemplateUri:        templatePath,
		TemplateHash:       common.HexToHash(templateHash),
		LastProcessedBlock: inputBoxBlockNumber,
		Status:             applicationStatus,
		IConsensusAddress:  authorityAddr,
	}

	_, err = cmdcommom.Database.InsertApplication(ctx, &application)
	cobra.CheckErr(err)
	fmt.Printf("Application %v successfully added\n", appAddr)
}

// FIXME remove this
func deployApplication(ctx context.Context, owner string, applicationFactoryAddr, authorityAddr common.Address, templateHash string, salt string) (common.Address, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	ownerAddr := common.HexToAddress(owner)
	templateHashBytes, err := hex.DecodeString(templateHash)
	if err != nil {
		log.Fatalf("Failed to decode template hash: %v", err)
	}
	saltBytes, err := hex.DecodeString(salt)
	if err != nil {
		log.Fatalf("Failed to decode salt: %v", err)
	}

	auth, err := getAuth(ctx, client)
	if err != nil {
		log.Fatalf("Failed to get transaction signer: %v", err)
	}

	factory, err := iapplicationfactory.NewIApplicationFactory(applicationFactoryAddr, client)
	if err != nil {
		log.Fatalf("Failed to instantiate contract: %v", err)
	}

	tx, err := factory.NewApplication(auth, authorityAddr, ownerAddr, toBytes32(templateHashBytes), toBytes32(saltBytes))
	if err != nil {
		log.Fatalf("Transaction failed: %v", err)
	}

	fmt.Printf("Transaction submitted: %s\n", tx.Hash().Hex())

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(context.Background(), client, tx)
	if err != nil {
		log.Fatalf("Failed to wait for transaction mining: %v", err)
	}

	if receipt.Status == 1 {
		fmt.Println("Transaction successful!")
	} else {
		log.Fatalf("Transaction failed!")
	}

	// Parse logs to get the address of the new application contract
	contractABI, err := abi.JSON(strings.NewReader(iapplicationfactory.IApplicationFactoryABI))
	if err != nil {
		log.Fatalf("Failed to parse ABI: %v", err)
	}

	// Look for the specific event in the receipt logs
	for _, vLog := range receipt.Logs {
		event := struct {
			Consensus    common.Address
			AppOwner     common.Address
			TemplateHash [32]byte
			AppContract  common.Address
		}{}

		// Parse log for ApplicationCreated event
		err := contractABI.UnpackIntoInterface(&event, "ApplicationCreated", vLog.Data)
		if err != nil {
			continue // Skip logs that don't match
		}

		fmt.Printf("New Application contract deployed at address: %s\n", event.AppContract.Hex())
		return event.AppContract, nil
	}

	return common.Address{}, fmt.Errorf("failed to find ApplicationCreated event in receipt logs")
}

// FIXME remove this
func deployAuthority(ctx context.Context, owner string, authorityFactoryAddr common.Address, epochLength uint64, salt string) (common.Address, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	ownerAddr := common.HexToAddress(owner)
	saltBytes, err := hex.DecodeString(salt)
	if err != nil {
		log.Fatalf("Failed to decode salt: %v", err)
	}

	auth, err := getAuth(ctx, client)
	if err != nil {
		log.Fatalf("Failed to get transaction signer: %v", err)
	}

	contract, err := iauthorityfactory.NewIAuthorityFactory(authorityFactoryAddr, client)
	if err != nil {
		log.Fatalf("Failed to instantiate contract: %v", err)
	}

	tx, err := contract.NewAuthority0(auth, ownerAddr, big.NewInt(int64(epochLength)), toBytes32(saltBytes))
	if err != nil {
		log.Fatalf("Transaction failed: %v", err)
	}

	fmt.Printf("Transaction submitted: %s\n", tx.Hash().Hex())

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(context.Background(), client, tx)
	if err != nil {
		log.Fatalf("Failed to wait for transaction mining: %v", err)
	}

	if receipt.Status == 1 {
		fmt.Println("Transaction successful!")
	} else {
		log.Fatalf("Transaction failed!")
	}

	// Parse logs to get the address of the new application contract
	contractABI, err := abi.JSON(strings.NewReader(iauthorityfactory.IAuthorityFactoryABI))
	if err != nil {
		log.Fatalf("Failed to parse ABI: %v", err)
	}

	// Look for the specific event in the receipt logs
	for _, vLog := range receipt.Logs {
		event := struct {
			Authority common.Address
		}{}

		// Parse log for ApplicationCreated event
		err := contractABI.UnpackIntoInterface(&event, "AuthorityCreated", vLog.Data)
		if err != nil {
			continue // Skip logs that don't match
		}

		fmt.Printf("New Authority contract deployed at address: %s\n", event.Authority.Hex())
		return event.Authority, nil
	}

	return common.Address{}, fmt.Errorf("failed to find AuthorityCreated event in receipt logs")
}

func getAuth(ctx context.Context, client *ethclient.Client) (*bind.TransactOpts, error) {
	var auth *bind.TransactOpts
	if privateKey != "" {
		key, err := crypto.HexToECDSA(privateKey)
		if err != nil {
			return nil, err
		}
		auth, err = bind.NewKeyedTransactorWithChainID(key, big.NewInt(1))
		if err != nil {
			return nil, err
		}
	} else if mnemonic != "" {
		signer, err := ethutil.NewMnemonicSigner(ctx, client, mnemonic, 0)
		if err != nil {
			return nil, err
		}
		auth, err = signer.MakeTransactor()
		if err != nil {
			return nil, err
		}
	} else {
		// Default private key (unsafe for production!)
		key, err := crypto.HexToECDSA("YOUR_DEFAULT_PRIVATE_KEY")
		if err != nil {
			return nil, err
		}
		auth, err = bind.NewKeyedTransactorWithChainID(key, big.NewInt(1))
		if err != nil {
			return nil, err
		}
	}
	return auth, nil
}

func toBytes32(data []byte) [32]byte {
	var arr [32]byte
	if len(data) != 32 {
		log.Fatalf("Invalid length: expected 32 bytes, got %d bytes", len(data))
	}
	copy(arr[:], data)
	return arr
}
