// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package register

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/app/util"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/dataavailability"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Cmd = &cobra.Command{
	Use:     "register",
	Short:   "Register an existing application on the node",
	Example: examples,
	Run:     run,
}

const examples = `# Adds an application to Rollups Node:
cartesi-rollups-cli app register -n echo-dapp -a 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF -t applications/echo-dapp`

var (
	name                   string
	applicationAddress     string
	consensusAddress       string
	templatePath           string
	templateHash           string
	epochLength            uint64
	inputBoxBlockNumber    uint64
	inputBoxAddress        string
	inputBoxAddressFromEnv bool
	dataAvailability       string
	blockchainHttpEndpoint string
	enableMachineHashCheck bool
	disabled               bool
	printAsJSON            bool
)

func init() {
	Cmd.Flags().StringVarP(&name, "name", "n", "", "Application name")
	cobra.CheckErr(Cmd.MarkFlagRequired("name"))

	Cmd.Flags().StringVarP(&applicationAddress, "address", "a", "", "Application contract address")
	cobra.CheckErr(Cmd.MarkFlagRequired("address"))

	Cmd.Flags().StringVarP(&consensusAddress, "consensus", "c", "",
		"Application IConsensus Address. (DO NOT USE IN PRODUCTION)\nThis value is retrieved from the application contract",
	)

	Cmd.Flags().StringVarP(&templatePath, "template-path", "t", "", "Application template URI")
	cobra.CheckErr(Cmd.MarkFlagRequired("template-path"))

	Cmd.Flags().StringVarP(&templateHash, "template-hash", "H", "",
		"Application template hash. (DO NOT USE IN PRODUCTION)\nThis value is retrieved from the application contract",
	)

	Cmd.Flags().Uint64VarP(&epochLength, "epoch-length", "e", 10, // nolint: mnd
		"Consensus Epoch length. (DO NOT USE IN PRODUCTION)\nThis value is retrieved from the consensus contract",
	)

	Cmd.Flags().StringVarP(&dataAvailability, "data-availability", "D", "",
		"Application ABI encoded Data Availability. If not provided, it will be read from the InputBox Address",
	)

	Cmd.Flags().StringVar(&inputBoxAddress, "inputbox", "", "Input Box contract address")
	cobra.CheckErr(viper.BindPFlag(config.CONTRACTS_INPUT_BOX_ADDRESS, Cmd.Flags().Lookup("inputbox")))

	Cmd.Flags().BoolVar(&inputBoxAddressFromEnv, "inputbox-from-env", false, "Read Input Box contract address from environment")
	Cmd.Flags().Uint64Var(&inputBoxBlockNumber, "inputbox-block-number", 0, "InputBox deployment block number")

	Cmd.Flags().BoolVarP(&disabled, "disabled", "d", false, "Sets the application state to disabled")

	Cmd.Flags().BoolVarP(&printAsJSON, "print-json", "j", false, "Prints the application data as JSON")

	Cmd.Flags().StringVar(&blockchainHttpEndpoint, "blockchain-http-endpoint", "", "Blockchain HTTP endpoint")
	cobra.CheckErr(viper.BindPFlag(config.BLOCKCHAIN_HTTP_ENDPOINT, Cmd.Flags().Lookup("blockchain-http-endpoint")))

	Cmd.Flags().BoolVar(&enableMachineHashCheck, "machine-hash-check", true,
		"Enable or disable machine hash check (DO NOT DISABLE IN PRODUCTION)")
	cobra.CheckErr(viper.BindPFlag(config.FEATURE_MACHINE_HASH_CHECK_ENABLED, Cmd.Flags().Lookup("machine-hash-check")))
}

func run(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	validName, err := config.ToApplicationNameFromString(name)
	cobra.CheckErr(err)

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.String())
	cobra.CheckErr(err)
	defer repo.Close()

	applicationState := model.ApplicationState_Enabled
	if disabled {
		applicationState = model.ApplicationState_Disabled
	}

	address := common.HexToAddress(applicationAddress)

	var parsedTemplateHash common.Hash
	if cmd.Flags().Changed("template-hash") {
		parsedTemplateHash = common.HexToHash(templateHash)
	} else {
		contractTemplateHash, err := getTemplateHash(ctx, address)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get template hash from application: %v\n", err)
			os.Exit(1)
		}
		parsedTemplateHash = *contractTemplateHash
		checkEnabled, err := config.GetFeatureMachineHashCheckEnabled()
		cobra.CheckErr(err)
		if checkEnabled {
			templateHash, err := util.ReadHash(templatePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Read machine template hash failed: %v\n", err)
				os.Exit(1)
			}
			snapshotTemplateHash := common.HexToHash(templateHash)
			if parsedTemplateHash != snapshotTemplateHash {
				fmt.Fprintf(os.Stderr, "Template hash mismatch: contract has %s but machine has %s\n",
					contractTemplateHash.Hex(), parsedTemplateHash.Hex())
				os.Exit(1)
			}
		}
	}

	var consensus common.Address
	if cmd.Flags().Changed("consensus") {
		consensus = common.HexToAddress(consensusAddress)
	} else {
		consensus, err = getConsensus(ctx, address)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get consensus address from application: %v\n", err)
			os.Exit(1)
		}
	}

	if !cmd.Flags().Changed("epoch-length") {
		epochLength, err = getEpochLength(ctx, consensus)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get epoch length from consensus: %v\n", err)
			os.Exit(1)
		}
	}

	inputBoxAddress, encodedDA, err := processDataAvailability(
		ctx,
		address,
		cmd.Flags().Changed("data-availability"),
		cmd.Flags().Changed("inputbox") || cmd.Flags().Changed("inputbox-from-env"),
	)
	cobra.CheckErr(err)

	var daSelector model.DataAvailabilitySelector
	copy(daSelector[:], encodedDA[:model.DATA_AVAILABILITY_SELECTOR_SIZE])

	if !cmd.Flags().Changed("inputbox-block-number") {
		block, err := getInputBoxDeploymentBlock(ctx, *inputBoxAddress)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get deployment block number: %v\n", err)
			os.Exit(1)
		}
		inputBoxBlockNumber = block.Uint64()
	}

	application := model.Application{
		Name:                 validName,
		IApplicationAddress:  address,
		IConsensusAddress:    consensus,
		IInputBoxAddress:     *inputBoxAddress,
		TemplateURI:          templatePath,
		TemplateHash:         parsedTemplateHash,
		EpochLength:          epochLength,
		DataAvailability:     daSelector,
		State:                applicationState,
		IInputBoxBlock:       inputBoxBlockNumber,
		LastInputCheckBlock:  0,
		LastOutputCheckBlock: 0,
	}

	_, err = repo.CreateApplication(ctx, &application)
	cobra.CheckErr(err)

	if printAsJSON {
		jsonData, err := json.Marshal(application)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshalling application to JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(jsonData))
	} else {
		fmt.Printf("Application %v successfully registered\n", application.IApplicationAddress)
	}
}

func getTemplateHash(
	ctx context.Context,
	appAddress common.Address,
) (*common.Hash, error) {
	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	cobra.CheckErr(err)
	client, err := ethclient.Dial(ethEndpoint.String())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint.Redacted())
	}
	return ethutil.GetTemplateHash(ctx, client, appAddress)
}

func getConsensus(
	ctx context.Context,
	appAddress common.Address,
) (common.Address, error) {
	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to get blockchain http endpoint address: %w", err)
	}
	client, err := ethclient.Dial(ethEndpoint.String())
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint.Redacted())
	}
	return ethutil.GetConsensus(ctx, client, appAddress)
}

func getEpochLength(
	ctx context.Context,
	consensusAddr common.Address,
) (uint64, error) {
	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return 0, fmt.Errorf("failed to get blockchain http endpoint address: %w", err)
	}
	client, err := ethclient.Dial(ethEndpoint.String())
	if err != nil {
		return 0, fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint.Redacted())
	}
	return ethutil.GetEpochLength(ctx, client, consensusAddr)
}

func getInputBoxDeploymentBlock(
	ctx context.Context,
	inputBoxAddress common.Address,
) (*big.Int, error) {
	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return nil, fmt.Errorf("failed to get blockchain http endpoint address: %w", err)
	}
	client, err := ethclient.Dial(ethEndpoint.String())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint.Redacted())
	}
	return ethutil.GetInputBoxDeploymentBlock(ctx, client, inputBoxAddress)
}

func getDataAvailability(
	ctx context.Context,
	appAddress common.Address,
) ([]byte, error) {
	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return nil, fmt.Errorf("failed to get blockchain http endpoint address: %w", err)
	}
	client, err := ethclient.Dial(ethEndpoint.String())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint.Redacted())
	}
	return ethutil.GetDataAvailability(ctx, client, appAddress)
}

func processDataAvailability(
	ctx context.Context,
	appAddress common.Address,
	hasDataAvailabilityFlag bool,
	hasInputBoxAddressFlag bool,
) (*common.Address, []byte, error) {
	var inputBoxAddress common.Address
	var encodedDA []byte
	var err error

	parsedAbi, err := dataavailability.DataAvailabilityMetaData.GetAbi()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get ABI: %w", err)
	}

	if hasInputBoxAddressFlag {
		inputBoxAddress, err = config.GetContractsInputBoxAddress()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get input box address: %w", err)
		}

		encodedDA, err = parsedAbi.Pack("InputBox", inputBoxAddress)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to pack InputBox: %w", err)
		}
	} else {
		if hasDataAvailabilityFlag {
			if len(dataAvailability) < 3 || (!strings.HasPrefix(dataAvailability, "0x") && !strings.HasPrefix(dataAvailability, "0X")) {
				return nil, nil, fmt.Errorf("data Availability should be an ABI encoded value")
			}

			s := dataAvailability[2:]
			encodedDA, err = hex.DecodeString(s)
			if err != nil {
				return nil, nil, fmt.Errorf("error parsing Data Availability value: %w", err)
			}
		} else {
			encodedDA, err = getDataAvailability(ctx, appAddress)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get Data Availability from Application: %w", err)
			}
		}

		if len(encodedDA) < model.DATA_AVAILABILITY_SELECTOR_SIZE {
			return nil, nil, fmt.Errorf("invalid Data Availability")
		}

		method, err := parsedAbi.MethodById(encodedDA[:model.DATA_AVAILABILITY_SELECTOR_SIZE])
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get method by ID: %w", err)
		}

		args, err := method.Inputs.Unpack(encodedDA[model.DATA_AVAILABILITY_SELECTOR_SIZE:])
		if err != nil {
			return nil, nil, fmt.Errorf("failed to unpack inputs: %w", err)
		}

		if len(args) == 0 {
			return nil, nil, fmt.Errorf("invalid Data Availability. Should at least contain InputBox Address")
		}

		switch addr := args[0].(type) {
		case common.Address:
			inputBoxAddress = addr
		default:
			return nil, nil, fmt.Errorf("first argument in Data Availability is not an address (got %T)", args[0])
		}
	}

	return &inputBoxAddress, encodedDA, nil
}
