// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"

	cmdcommom "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/advancer/snapshot"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplicationfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iauthorityfactory"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
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
cartesi-rollups-cli app deploy -n echo-dapp -a 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF -c 0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA -t applications/echo-dapp` //nolint:lll

var (
	name                 string
	applicationOwner     string
	authorityOwner       string
	templatePath         string
	templateHash         string
	consensusAddr        string
	appFactoryAddr       string
	authorityFactoryAddr string
	rpcURL               string
	privateKey           string
	mnemonic             string
	salt                 string
	inputBoxBlockNumber  uint64
	epochLength          uint64
	disabled             bool
	printAsJSON          bool
	noRegister           bool
)

func init() {
	Cmd.Flags().StringVarP(
		&name,
		"name",
		"n",
		"",
		"Application name",
	)
	cobra.CheckErr(Cmd.MarkFlagRequired("name"))

	Cmd.Flags().StringVarP(
		&applicationOwner,
		"app-owner",
		"o",
		"0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
		"Application owner",
	)

	Cmd.Flags().StringVarP(
		&authorityOwner,
		"authority-owner",
		"O",
		"0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
		"Authority owner",
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
		&templateHash,
		"template-hash",
		"H",
		"",
		"Application template hash. If not provided, it will be read from the template URI",
	)

	Cmd.Flags().BoolVarP(
		&disabled,
		"disabled",
		"d",
		false,
		"Sets the application state to disabled",
	)

	Cmd.Flags().StringVarP(
		&appFactoryAddr,
		"app-factory",
		"a",
		"0xd7d4d184b82b1a4e08f304DDaB0A2A7a301C2620",
		"Application Factory Address",
	)

	Cmd.Flags().StringVarP(
		&authorityFactoryAddr,
		"authority-factory",
		"C",
		"0xB897F7Fe78f220aE34B7FA9493092701a873Ed45",
		"Authority Factory Address",
	)

	Cmd.Flags().StringVarP(
		&consensusAddr,
		"consensus",
		"c",
		"",
		"Application IConsensus Address",
	)

	Cmd.Flags().Uint64VarP(
		&epochLength,
		"epoch-length",
		"e",
		10,
		"Consensus Epoch length. If consensus address is provided, the value will be read from the contract",
	)

	Cmd.Flags().StringVarP(&rpcURL, "rpc-url", "r", "http://localhost:8545", "Ethereum RPC URL")
	Cmd.Flags().StringVarP(&privateKey, "private-key", "k", "", "Private key for signing transactions")
	Cmd.Flags().StringVarP(&mnemonic, "mnemonic", "m", ethutil.FoundryMnemonic, "Mnemonic for signing transactions")
	Cmd.Flags().StringVar(&salt, "salt", "0000000000000000000000000000000000000000000000000000000000000000", "salt")
	Cmd.Flags().Uint64VarP(&inputBoxBlockNumber, "inputbox-block-number", "i", 0, "InputBox deployment block number")
	Cmd.Flags().BoolVarP(&printAsJSON, "print-json", "j", false, "Prints the application data as JSON")
	Cmd.Flags().BoolVar(&noRegister, "no-register", false, "Don't register the application on the node. Only deploy contracts")
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	if cmdcommom.Repository == nil {
		panic("Database was not initialized")
	}

	applicationState := model.ApplicationState_Enabled
	if disabled {
		applicationState = model.ApplicationState_Disabled
	}

	if templateHash == "" {
		var err error
		templateHash, err = snapshot.ReadHash(templatePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Read machine template hash failed: %v\n", err)
			os.Exit(1)
		}
	}

	var consensus common.Address
	var err error
	if consensusAddr == "" {
		authorityFactoryAddress := common.HexToAddress(authorityFactoryAddr)
		consensus, err = deployAuthority(ctx, authorityOwner, authorityFactoryAddress, epochLength, salt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Authoriy contract creation failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		consensus = common.HexToAddress(consensusAddr)
		epochLength, err = getEpochLength(consensus)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get epoch length from consensus: %v\n", err)
			os.Exit(1)
		}
	}

	applicationFactoryAddress := common.HexToAddress(appFactoryAddr)
	appAddr, err := deployApplication(ctx, applicationOwner, applicationFactoryAddress, consensus, templateHash, salt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Application contract creation failed: %v\n", err)
		os.Exit(1)
	}

	application := model.Application{
		Name:                 name,
		IApplicationAddress:  appAddr,
		IConsensusAddress:    consensus,
		TemplateURI:          templatePath,
		TemplateHash:         common.HexToHash(templateHash),
		EpochLength:          epochLength,
		State:                applicationState,
		LastProcessedBlock:   inputBoxBlockNumber,
		LastOutputCheckBlock: inputBoxBlockNumber,
		LastClaimCheckBlock:  inputBoxBlockNumber,
	}

	if !noRegister {
		_, err = cmdcommom.Repository.CreateApplication(ctx, &application)
		cobra.CheckErr(err)
	}

	if printAsJSON {
		jsonData, err := json.Marshal(application)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshalling application to JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(jsonData))
	} else {
		fmt.Printf("Application %v successfully deployed\n", application.IApplicationAddress)
	}
}

// FIXME remove this
func deployApplication(
	ctx context.Context,
	owner string,
	applicationFactoryAddr,
	authorityAddr common.Address,
	templateHash string,
	salt string,
) (common.Address, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to connect to the Ethereum client: %v", err)
	}

	ownerAddr := common.HexToAddress(owner)
	templateHashBytes, err := hex.DecodeString(templateHash)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to decode template hash: %v", err)
	}
	saltBytes, err := hex.DecodeString(salt)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to decode salt: %v", err)
	}

	auth, err := getAuth(ctx, client)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to get transaction signer: %v", err)
	}

	factory, err := iapplicationfactory.NewIApplicationFactory(applicationFactoryAddr, client)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to instantiate contract: %v", err)
	}

	tx, err := factory.NewApplication(auth, authorityAddr, ownerAddr, toBytes32(templateHashBytes), toBytes32(saltBytes))
	if err != nil {
		return common.Address{}, fmt.Errorf("Transaction failed: %v", err)
	}

	if !printAsJSON {
		fmt.Printf("Transaction submitted: %s\n", tx.Hash().Hex())
	}

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(context.Background(), client, tx)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to wait for transaction mining: %v", err)
	}

	if receipt.Status == 1 {
		if !printAsJSON {
			fmt.Println("Transaction successful!")
		}
	} else {
		return common.Address{}, fmt.Errorf("Transaction failed!")
	}

	// Parse logs to get the address of the new application contract
	contractABI, err := abi.JSON(strings.NewReader(iapplicationfactory.IApplicationFactoryABI))
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to parse ABI: %v", err)
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

		if !printAsJSON {
			fmt.Printf("New Application contract deployed at address: %s\n", event.AppContract.Hex())
		}
		return event.AppContract, nil
	}

	return common.Address{}, fmt.Errorf("failed to find ApplicationCreated event in receipt logs")
}

// FIXME remove this
func deployAuthority(
	ctx context.Context,
	owner string,
	authorityFactoryAddr common.Address,
	epochLength uint64,
	salt string,
) (common.Address, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to connect to the Ethereum client: %v", err)
	}

	ownerAddr := common.HexToAddress(owner)
	saltBytes, err := hex.DecodeString(salt)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to decode salt: %v", err)
	}

	auth, err := getAuth(ctx, client)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to get transaction signer: %v", err)
	}

	contract, err := iauthorityfactory.NewIAuthorityFactory(authorityFactoryAddr, client)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to instantiate contract: %v", err)
	}

	tx, err := contract.NewAuthority0(auth, ownerAddr, big.NewInt(int64(epochLength)), toBytes32(saltBytes))
	if err != nil {
		return common.Address{}, fmt.Errorf("Transaction failed: %v", err)
	}

	if !printAsJSON {
		fmt.Printf("Transaction submitted: %s\n", tx.Hash().Hex())
	}

	// Wait for the transaction to be mined
	receipt, err := bind.WaitMined(context.Background(), client, tx)
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to wait for transaction mining: %v", err)
	}

	if receipt.Status == 1 {
		if !printAsJSON {
			fmt.Println("Transaction successful!")
		}
	} else {
		return common.Address{}, fmt.Errorf("Transaction failed!")
	}

	// Parse logs to get the address of the new application contract
	contractABI, err := abi.JSON(strings.NewReader(iauthorityfactory.IAuthorityFactoryABI))
	if err != nil {
		return common.Address{}, fmt.Errorf("Failed to parse ABI: %v", err)
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

		if !printAsJSON {
			fmt.Printf("New Authority contract deployed at address: %s\n", event.Authority.Hex())
		}
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
		chainId, err := client.ChainID(ctx)
		if err != nil {
			return nil, fmt.Errorf("Failed to get chain id: %v", err)
		}
		auth, err = bind.NewKeyedTransactorWithChainID(key, chainId)
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

func getEpochLength(
	consensusAddr common.Address,
) (uint64, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return 0, fmt.Errorf("Failed to connect to the Ethereum client: %v", err)
	}

	consensus, err := iconsensus.NewIConsensus(consensusAddr, client)
	if err != nil {
		return 0, fmt.Errorf("Failed to instantiate contract: %v", err)
	}

	epochLengthRaw, err := consensus.GetEpochLength(nil)
	if err != nil {
		return 0, fmt.Errorf("error retrieving application epoch length: %v", err)
	}
	return epochLengthRaw.Uint64(), nil
}

func toBytes32(data []byte) [32]byte {
	var arr [32]byte
	if len(data) != 32 {
		fmt.Fprintf(os.Stderr, "Invalid length: expected 32 bytes, got %d bytes", len(data))
		os.Exit(1)
	}
	copy(arr[:], data)
	return arr
}
