// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package register

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/util"
	"github.com/cartesi/rollups-node/internal/cli"
	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/iapplication"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/contracts/iquorum"
	"github.com/cartesi/rollups-node/pkg/ethutil"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
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
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string
  CARTESI_BLOCKCHAIN_HTTP_ENDPOINT               Blockchain HTTP endpoint
  CARTESI_FEATURE_MACHINE_HASH_CHECK_ENABLED     Enable machine hash check`,
}

const examples = `# Adds an application to Rollups Node:
cartesi-rollups-cli app register -n echo-dapp -a 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF -t applications/echo-dapp`

var (
	name                         string
	applicationAddress           string
	consensusAddress             string
	templatePath                 string
	templateHash                 string
	epochLength                  uint64
	claimStagingPeriod           uint64
	enableMachineHashCheck       bool
	applicationTypePRT           bool
	disabled                     bool
	printAsJSON                  bool
	executionParametersFileParam string
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

	Cmd.Flags().Uint64VarP(&epochLength, "epoch-length", "e", 0,
		"Consensus Epoch length. (DO NOT USE IN PRODUCTION)\nThis value is retrieved from the consensus contract",
	)

	Cmd.Flags().Uint64Var(&claimStagingPeriod, "claim-staging-period", 0,
		"Consensus claim staging period in blocks. "+
			"(DO NOT USE IN PRODUCTION)\nThis value is retrieved from the consensus contract",
	)

	Cmd.Flags().BoolVarP(&disabled, "disabled", "d", false, "Registers the application with enabled=false")

	Cmd.Flags().BoolVarP(&printAsJSON, "print-json", "j", false, "Prints the application data as JSON")

	Cmd.Flags().StringVarP(&executionParametersFileParam, "execution-parameters-file", "", "",
		"JSON encoded execution parameters of the application. Default values will be used if not defined.")

	Cmd.Flags().BoolVar(&enableMachineHashCheck, "machine-hash-check", true,
		"Enable or disable machine hash check (DO NOT DISABLE IN PRODUCTION)")
	cobra.CheckErr(viper.BindPFlag(config.FEATURE_MACHINE_HASH_CHECK_ENABLED, Cmd.Flags().Lookup("machine-hash-check")))

	Cmd.Flags().BoolVarP(&applicationTypePRT, "prt", "", false, "Register as PRT application.")

	origHelpFunc := Cmd.HelpFunc()
	Cmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		command.Flags().Lookup("blockchain-http-endpoint").Hidden = false
		origHelpFunc(command, strings)
	})
}

func run(cmd *cobra.Command, _ []string) {
	ctx := cmd.Context()

	validName, err := config.ToApplicationNameFromString(name)
	cobra.CheckErr(err)

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	cobra.CheckErr(err)
	defer repo.Close()

	applicationEnabled := !disabled

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
			snapshotTemplateHash, err := util.ReadRootHash(templatePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Read machine template hash failed: %v\n", err)
				os.Exit(1)
			}
			if parsedTemplateHash != snapshotTemplateHash {
				fmt.Fprintf(os.Stderr, "Template hash mismatch: contract has %s but machine has %s\n",
					parsedTemplateHash.Hex(), snapshotTemplateHash.Hex())
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
			fmt.Fprintf(os.Stderr, "Failed to get consensus address from application: %v\n",
				cli.DecorateRevert(err, iapplication.IApplicationMetaData))
			os.Exit(1)
		}
	}

	if !cmd.Flags().Changed("epoch-length") && !applicationTypePRT {
		epochLength, err = getEpochLength(ctx, consensus)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get epoch length from consensus: %v\n",
				cli.DecorateRevert(err, iconsensus.IConsensusMetaData))
			repo.Close()
			os.Exit(1) //nolint:gocritic // The repository is closed explicitly before exiting.
		}
	}

	if !cmd.Flags().Changed("claim-staging-period") {
		claimStagingPeriod, err = getClaimStagingPeriod(ctx, consensus)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get claim staging period from consensus: %v\n",
				cli.DecorateRevert(err, iconsensus.IConsensusMetaData))
			os.Exit(1)
		}
	}

	withdrawalConfig, err := readApplicationWithdrawalConfig(ctx, address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read withdrawal config from application: %v\n",
			cli.DecorateRevert(err, iapplication.IApplicationMetaData))
		os.Exit(1)
	}

	inputBoxAddress, err := getInputBox(ctx, address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get input box address from application: %v\n",
			cli.DecorateRevert(err, iapplication.IApplicationMetaData))
		os.Exit(1)
	}

	block, err := getInputBoxDeploymentBlock(ctx, inputBoxAddress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get deployment block number: %v\n", err)
		os.Exit(1)
	}
	if !block.IsUint64() {
		fmt.Fprintf(os.Stderr, "Input box deployment block does not fit uint64: %v\n", block)
		os.Exit(1)
	}
	inputBoxBlockNumber := block.Uint64()

	// ensure there is a contract deployed at the input box address
	hasCode, err := hasCodeAt(ctx, inputBoxAddress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to probe input box address for contract: %v\n", err)
		os.Exit(1)
	}
	if !hasCode {
		fmt.Fprintf(os.Stderr, "input box address has no code: %v\n", inputBoxAddress)
		os.Exit(1)
	}

	consensusType, err := getConsensusType(ctx, consensus, applicationTypePRT)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to detect consensus type: %v\n", err)
		os.Exit(1)
	}

	application := model.Application{
		Name:                              validName,
		IApplicationAddress:               address,
		IConsensusAddress:                 consensus,
		IInputBoxAddress:                  inputBoxAddress,
		TemplateURI:                       templatePath,
		TemplateHash:                      parsedTemplateHash,
		EpochLength:                       epochLength,
		ClaimStagingPeriod:                claimStagingPeriod,
		WithdrawalConfig:                  withdrawalConfig,
		ConsensusType:                     consensusType,
		Enabled:                           applicationEnabled,
		Status:                            model.ApplicationStatus_OK,
		IInputBoxBlock:                    inputBoxBlockNumber,
		LastEpochCheckBlock:               0,
		LastInputCheckBlock:               0,
		LastOutputCheckBlock:              0,
		LastTournamentCheckBlock:          0,
		LastForecloseCheckBlock:           0,
		LastAccountsDriveProvedCheckBlock: 0,
		LastWithdrawalCheckBlock:          0,
	}

	// load execution parameters from a file?
	withExecutionParameters := cmd.Flags().Changed("execution-parameters-file")
	if withExecutionParameters {
		filePath := executionParametersFileParam
		if executionParametersFileParam == "-" {
			filePath = os.Stdin.Name()
		}
		contents, err := os.ReadFile(filePath) //nolint:gosec // The CLI user explicitly supplies this path.
		cobra.CheckErr(err)

		decoder := json.NewDecoder(strings.NewReader(string(contents)))
		decoder.DisallowUnknownFields() // Prevent unexpected fields
		err = decoder.Decode(&application.ExecutionParameters)
		cobra.CheckErr(err)
		cobra.CheckErr(application.ExecutionParameters.Validate())
	}

	_, err = repo.CreateApplication(ctx, &application, withExecutionParameters)
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
	client, err := ethclient.Dial(ethEndpoint.Raw())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint)
	}
	return ethutil.GetTemplateHash(ctx, client, appAddress)
}

func hasCodeAt(
	ctx context.Context,
	consensusAddress common.Address,
) (bool, error) {
	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return false, fmt.Errorf("failed to get blockchain http endpoint address: %w", err)
	}
	client, err := ethclient.Dial(ethEndpoint.Raw())
	if err != nil {
		return false, fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint)
	}
	bytes, err := client.CodeAt(ctx, consensusAddress, nil)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve code at consensus address: %s", consensusAddress)
	}
	return len(bytes) != 0, nil
}

func getConsensus(
	ctx context.Context,
	appAddress common.Address,
) (common.Address, error) {
	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to get blockchain http endpoint address: %w", err)
	}
	client, err := ethclient.Dial(ethEndpoint.Raw())
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint)
	}
	return ethutil.GetConsensus(ctx, client, appAddress)
}

func getInputBox(
	ctx context.Context,
	appAddress common.Address,
) (common.Address, error) {
	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to get blockchain http endpoint address: %w", err)
	}
	client, err := ethclient.Dial(ethEndpoint.Raw())
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint)
	}
	return ethutil.GetInputBox(ctx, client, appAddress)
}

func getEpochLength(
	ctx context.Context,
	consensusAddr common.Address,
) (uint64, error) {
	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return 0, fmt.Errorf("failed to get blockchain http endpoint address: %w", err)
	}
	client, err := ethclient.Dial(ethEndpoint.Raw())
	if err != nil {
		return 0, fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint)
	}
	return ethutil.GetEpochLength(ctx, client, consensusAddr)
}

func getClaimStagingPeriod(
	ctx context.Context,
	consensusAddr common.Address,
) (uint64, error) {
	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return 0, fmt.Errorf("failed to get blockchain http endpoint address: %w", err)
	}
	client, err := ethclient.Dial(ethEndpoint.Raw())
	if err != nil {
		return 0, fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint)
	}
	return ethutil.GetClaimStagingPeriod(ctx, client, consensusAddr)
}

type quorumConsensusProbe interface {
	NumOfValidators(opts *bind.CallOpts) (*big.Int, error)
}

func getConsensusType(
	ctx context.Context,
	consensusAddr common.Address,
	applicationTypePRT bool,
) (model.Consensus, error) {
	if applicationTypePRT {
		return model.Consensus_PRT, nil
	}

	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return "", fmt.Errorf("failed to get blockchain http endpoint address: %w", err)
	}
	client, err := ethclient.DialContext(ctx, ethEndpoint.Raw())
	if err != nil {
		return "", fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint)
	}
	quorum, err := iquorum.NewIQuorum(consensusAddr, client)
	if err != nil {
		return "", err
	}
	return consensusTypeFromQuorumProbe(applicationTypePRT, quorum)
}

func consensusTypeFromQuorumProbe(
	applicationTypePRT bool,
	probe quorumConsensusProbe,
) (model.Consensus, error) {
	if applicationTypePRT {
		return model.Consensus_PRT, nil
	}
	numOfValidators, err := probe.NumOfValidators(nil)
	if err != nil {
		return model.Consensus_Authority, nil
	}
	if numOfValidators == nil || numOfValidators.Sign() == 0 {
		return "", fmt.Errorf("quorum consensus reports zero validators")
	}
	return model.Consensus_Quorum, nil
}

func readApplicationWithdrawalConfig(
	ctx context.Context,
	appAddr common.Address,
) (model.WithdrawalConfig, error) {
	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return model.WithdrawalConfig{}, fmt.Errorf("failed to get blockchain http endpoint address: %w", err)
	}
	client, err := ethclient.Dial(ethEndpoint.Raw())
	if err != nil {
		return model.WithdrawalConfig{}, fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint)
	}
	wc, err := ethutil.GetApplicationWithdrawalConfig(ctx, client, appAddr)
	if err != nil {
		return model.WithdrawalConfig{}, err
	}
	return model.WithdrawalConfig(wc), nil
}

func getInputBoxDeploymentBlock(
	ctx context.Context,
	inputBoxAddress common.Address,
) (*big.Int, error) {
	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	if err != nil {
		return nil, fmt.Errorf("failed to get blockchain http endpoint address: %w", err)
	}
	client, err := ethclient.Dial(ethEndpoint.Raw())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the blockchain http endpoint: %s", ethEndpoint)
	}
	return ethutil.GetInputBoxDeploymentBlock(ctx, client, inputBoxAddress)
}
