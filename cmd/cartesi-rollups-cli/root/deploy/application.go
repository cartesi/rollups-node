// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/config/auth"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"
	"github.com/cartesi/rollups-node/pkg/contracts/iconsensus"
	"github.com/cartesi/rollups-node/pkg/ethutil"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/cobra"
)

var (
	applicationConsensusAddressParam string
	applicationDataAvailabilityParam string
	applicationEnableParam           bool
	applicationOwnerAddressParam     string
	applicationRegisterParam         bool
	applicationTemplateHashParam     string
	factoryAddressParam              string
	executionParametersFileParam     string
	prtFactoryAddressParam           string
	deploymentTypePRT                bool
)

var applicationCmd = &cobra.Command{
	Use:   "application [application-name] [template-path]",
	Short: "Deploy a new application and register it into the database",

	Args: func(cmd *cobra.Command, args []string) error {
		if !(0 <= len(args) && len(args) <= 2) {
			return fmt.Errorf("error on argument count. Expected at most two positional arguments")
		}
		return cobra.OnlyValidArgs(cmd, args)
	},
	Example: applicationExamples,
	Run:     runDeployApplication,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                                Database connection string
  CARTESI_BLOCKCHAIN_HTTP_ENDPOINT                           Blockchain HTTP endpoint
  CARTESI_CONTRACTS_INPUT_BOX_ADDRESS                        Input Box contract address
  CARTESI_CONTRACTS_APPLICATION_FACTORY_ADDRESS              Application Factory address
  CARTESI_CONTRACTS_SELF_HOSTED_APPLICATION_FACTORY_ADDRESS  Self Hosted Application Factory address
  CARTESI_CONTRACTS_PRT_CONSENSUS_FACTORY_ADDRESS            PRT Factory address`,
}

const applicationExamples = `
# deploy both application and authority contracts together via self hosted application contract, then register the application
 - cli deploy application echo-dapp applications/echo-dapp/

# deploy an application contract using an existing consensus, then register the application
 - cli deploy application echo-dapp applications/echo-dapp/ --consensus=0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA

# deploy an application contract with a PRT consensus, then register the application
 - cli deploy application echo-dapp applications/echo-dapp/ --prt

# deploy but don't register into the database
 - cli deploy application echo-dapp applications/echo-dapp/ --register=false

# deploy and register into the database, but disabled
 - cli deploy application echo-dapp applications/echo-dapp/ --enable=false

# deploy an application without a machine template path (both application-name and template-path may be omitted in this case)
 - cli deploy application --template-hash=0x0000000000000000000000000000000000000000000000000000000000000000 --register=false`

func init() {
	applicationCmd.Flags().StringVarP(&applicationConsensusAddressParam, "consensus", "c", "",
		"Consensus address. A new authority consensus will be created if this field is left empty.")
	applicationCmd.Flags().StringVarP(&factoryAddressParam, "factory", "f", "",
		"Application factory address. Default value is retrieved from configuration.")
	applicationCmd.Flags().StringVarP(&prtFactoryAddressParam, "prt-factory", "", "",
		"PRT Application factory address. Default value is retrieved from configuration.")
	applicationCmd.Flags().StringVarP(&applicationOwnerAddressParam, "application-owner", "o", "",
		"Application owner address. If not defined, it will be derived from the auth method.")
	applicationCmd.Flags().StringVarP(&applicationDataAvailabilityParam, "data-availability", "d", "",
		"Data availability string. Default is input box.")
	applicationCmd.Flags().StringVarP(&applicationTemplateHashParam, "template-hash", "H", "",
		"Template hash. If not provided, it will be read from the template path")
	applicationCmd.Flags().BoolVarP(&applicationRegisterParam, "register", "r", true,
		"Register the application into the database")
	applicationCmd.Flags().StringVarP(&executionParametersFileParam, "execution-parameters-file", "", "",
		"JSON encoded execution parameters of the application. Default values will be used if not defined.")
	applicationCmd.Flags().BoolVarP(&applicationEnableParam, "enable", "e", true,
		"Start processing the application, requires 'register=true'.")
	applicationCmd.Flags().StringVarP(&authorityOwnerAddressParam, "authority-owner", "O", "",
		"Authority Owner address. If not defined, it will be derived from the auth method.")
	applicationCmd.Flags().BoolVarP(&deploymentTypePRT, "prt", "", false,
		"Deploy a PRT application.")

	origHelpFunc := applicationCmd.HelpFunc()
	applicationCmd.SetHelpFunc(func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("epoch-length").Hidden = false
		command.Flags().Lookup("salt").Hidden = false
		command.Flags().Lookup("json").Hidden = false
		command.Flags().Lookup("verbose").Hidden = false
		origHelpFunc(command, strings)
	})
}

func runDeployApplication(cmd *cobra.Command, args []string) {
	applicationName := ""
	templateURI := ""

	ctx := cmd.Context()

	ethEndpoint, err := config.GetBlockchainHttpEndpoint()
	cobra.CheckErr(err)

	client, err := ethclient.DialContext(ctx, ethEndpoint.String())
	cobra.CheckErr(err)

	chainId, err := client.ChainID(ctx)
	cobra.CheckErr(err)

	txOpts, err := auth.GetTransactOpts(chainId)
	cobra.CheckErr(err)

	// pre deployment checks
	if len(args) >= 1 {
		applicationName = args[0]
	}
	if len(args) >= 2 {
		templateURI = args[1]
	}

	if applicationRegisterParam {
		// check if name is valid and available before deploying
		applicationName, err = config.ToApplicationNameFromString(applicationName)
		cobra.CheckErr(err)

		dsn, err := config.GetDatabaseConnection()
		repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.String())
		cobra.CheckErr(err)
		defer repo.Close()

		applicationInUse, err := repo.GetApplication(ctx, applicationName)
		cobra.CheckErr(err)

		if applicationInUse != nil {
			cobra.CheckErr(fmt.Errorf("application name is already in use: %v.", applicationInUse.Name))
		}
	}

	var deployment ethutil.IApplicationDeployment
	if deploymentTypePRT {
		deployment, err = buildPrtApplicationDeployment(ctx, cmd, args, client, txOpts)
	} else if deploySelfhosted := !cmd.Flags().Changed("consensus"); deploySelfhosted {
		deployment, err = buildSelfhostedApplicationDeployment(ctx, cmd, args, client, txOpts)
	} else {
		deployment, err = buildApplicationOnlyDeployment(ctx, cmd, args, client, txOpts)
	}
	cobra.CheckErr(err)

	if verboseParam {
		fmt.Fprint(os.Stderr, deployment)
		fmt.Fprintln(os.Stderr, "\twallet address:       ", txOpts.From)
	}

	application := model.Application{}
	executionParameters := model.ExecutionParameters{}

	application.Name = applicationName
	application.TemplateURI = templateURI
	application.State = model.ApplicationState_Disabled
	if applicationEnableParam {
		application.State = model.ApplicationState_Enabled
	}

	if changed := cmd.Flags().Changed("execution-parameters-file"); changed {
		filePath := executionParametersFileParam
		if executionParametersFileParam == "-" {
			filePath = os.Stdin.Name()
		}
		contents, err := os.ReadFile(filePath)
		cobra.CheckErr(err)

		decoder := json.NewDecoder(strings.NewReader(string(contents)))
		decoder.DisallowUnknownFields() // Prevent unexpected fields
		err = decoder.Decode(&executionParameters)
		cobra.CheckErr(err)
		cobra.CheckErr(executionParameters.Validate())

		if verboseParam {
			fmt.Fprintln(os.Stderr, "loading execution parameters...success")
		}
	}

	// factory check
	if verboseParam {
		fmt.Fprint(os.Stderr, "checking factory address...")
	}

	factoryAddress := deployment.GetFactoryAddress()
	data, err := client.CodeAt(ctx, factoryAddress, nil)
	cobra.CheckErr(err)

	if len(data) == 0 {
		cobra.CheckErr(fmt.Errorf("No code at the factory address: %v", factoryAddress))
	}
	if verboseParam {
		fmt.Fprint(os.Stderr, "success\n")
	}

	// deploy
	if verboseParam || !asJsonParam {
		fmt.Fprint(os.Stderr, "deploying...")
	}
	_, result, err := deployment.Deploy(ctx, client, txOpts)
	cobra.CheckErr(err)

	if verboseParam || !asJsonParam {
		fmt.Fprint(os.Stderr, "success\n")
		fmt.Fprint(os.Stderr, result)
	}

	// TODO(mpolitzer): can this be more concise?
	// (they are similar but different in a couple of fields)
	switch res := result.(type) {
	case *ethutil.SelfhostedApplicationDeploymentResult:
		application.IApplicationAddress = res.ApplicationAddress
		application.IConsensusAddress = res.AuthorityAddress
		application.IInputBoxAddress = res.Deployment.InputBoxAddress
		application.TemplateHash = res.Deployment.TemplateHash
		application.EpochLength = res.Deployment.EpochLength
		application.DataAvailability = res.Deployment.DataAvailability
		application.IInputBoxBlock = res.Deployment.IInputBoxBlock

	case *ethutil.ApplicationDeploymentResult:
		application.IApplicationAddress = res.ApplicationAddress
		application.IConsensusAddress = res.Deployment.Consensus
		application.IInputBoxAddress = res.Deployment.InputBoxAddress
		application.TemplateHash = res.Deployment.TemplateHash
		application.EpochLength = res.Deployment.EpochLength
		application.DataAvailability = res.Deployment.DataAvailability
		application.IInputBoxBlock = res.Deployment.IInputBoxBlock

	case *ethutil.PRTApplicationDeploymentResult:
		application.IApplicationAddress = res.ApplicationResult.ApplicationAddress
		application.IConsensusAddress = res.ApplicationResult.Deployment.Consensus
		application.IInputBoxAddress = res.ApplicationResult.Deployment.InputBoxAddress
		application.TemplateHash = res.ApplicationResult.Deployment.TemplateHash
		application.EpochLength = res.ApplicationResult.Deployment.EpochLength
		application.DataAvailability = res.ApplicationResult.Deployment.DataAvailability
		application.IInputBoxBlock = res.ApplicationResult.Deployment.IInputBoxBlock
		application.DaveConsensus = true
	default:
		panic("unimplemented deployment type\n")
	}

	if applicationRegisterParam {
		if verboseParam || !asJsonParam {
			fmt.Fprint(os.Stderr, "registering...")
		}
		dsn, err := config.GetDatabaseConnection()
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to register application: %w", err))
		}

		repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.String())
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to register application: %w", err))
		}
		defer repo.Close()

		_, err = repo.CreateApplication(ctx, &application)
		if err != nil {
			cobra.CheckErr(fmt.Errorf("failed to register application: %w", err))
		}

		// retrieve fields initialized by the database
		retrievedApplication, err := repo.GetApplication(ctx, applicationName)
		if verboseParam || !asJsonParam {
			if err != nil {
				fmt.Fprint(os.Stderr, "success, but failed to retrieve the database initialized fields. Will display them as placeholders.\n")
			} else {
				application = *retrievedApplication
				fmt.Fprint(os.Stderr, "success\n")
			}
		}
		if changed := cmd.Flags().Changed("execution-parameters-file"); changed {
			executionParameters.ApplicationID = application.ID
			err = repo.UpdateExecutionParameters(ctx, &executionParameters)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to update execution parameters: %w", err))
			}
		}

		if verboseParam || !asJsonParam {
			if applicationName != "" || verboseParam {
				fmt.Fprintln(os.Stderr, "\tapplication name:     ", applicationName)
			}
			if templateURI != "" || verboseParam {
				fmt.Fprintln(os.Stderr, "\tapplication path:     ", templateURI)
			}
		}
	} else if verboseParam {
		fmt.Fprint(os.Stderr, "registering...skipped\n")
	}

	if asJsonParam {
		report, err := json.MarshalIndent(&application, "", "  ")
		cobra.CheckErr(err)
		fmt.Println(string(report))
	}
}

// parse args + cmd into a self hosted deployment structure
func buildSelfhostedApplicationDeployment(
	ctx context.Context,
	cmd *cobra.Command,
	args []string,
	client *ethclient.Client,
	txOpts *bind.TransactOpts,
) (*ethutil.SelfhostedApplicationDeployment, error) {
	var err error
	request := &ethutil.SelfhostedApplicationDeployment{}

	if !cmd.Flags().Changed("factory") {
		request.FactoryAddress, err = config.GetContractsSelfHostedApplicationFactoryAddress()
	} else {
		request.FactoryAddress, err = parseHexAddress(factoryAddressParam)
	}
	if err != nil {
		return nil, fmt.Errorf("error on parameter selfhosted-factory: %w", err)
	}

	if !cmd.Flags().Changed("application-owner") {
		request.ApplicationOwnerAddress = txOpts.From
	} else {
		request.ApplicationOwnerAddress, err = parseHexAddress(applicationOwnerAddressParam)
	}
	if err != nil {
		return nil, fmt.Errorf("error on parameter application-owner: %w", err)
	}

	if !cmd.Flags().Changed("authority-owner") {
		request.AuthorityOwnerAddress = txOpts.From
	} else {
		request.AuthorityOwnerAddress, err = parseHexAddress(authorityOwnerAddressParam)
	}
	if err != nil {
		return nil, fmt.Errorf("error on parameter authority-owner: %w", err)
	}

	if !cmd.Flags().Changed("template-hash") {
		if len(args) >= 2 { // args[1] is mandatory if `template-hash` was absent
			request.TemplateHash, err = readHash(args[1])
		} else {
			err = fmt.Errorf("missing argument. One of `template-path` or `template-hash` is required")
		}
	} else {
		request.TemplateHash, err = parseHexHash(applicationTemplateHashParam)
	}
	if err != nil {
		return nil, fmt.Errorf("error on parameter template-hash: %w", err)
	}

	if !cmd.Flags().Changed("data-availability") {
		inputBoxAddress := common.Address{}
		inputBoxAddress, err = config.GetContractsInputBoxAddress()
		if err != nil {
			return nil, fmt.Errorf("error on parameter data-availability: %w", err)
		}
		request.InputBoxAddress, request.IInputBoxBlock, request.DataAvailability, err =
			ethutil.DefaultDA(client, inputBoxAddress)
	} else {
		request.InputBoxAddress, request.IInputBoxBlock, request.DataAvailability, err =
			ethutil.CustomDA(client, applicationDataAvailabilityParam)
	}
	if err != nil {
		return nil, fmt.Errorf("error on parameter data-availability: %w", err)
	}

	// ensure there is a contract deployed at the input box address
	code, err := client.CodeAt(ctx, request.InputBoxAddress, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to probe input box address for contract: %v\n", err)
	}
	if len(code) == 0 {
		return nil, fmt.Errorf("error input box address has no code: %v", request.InputBoxAddress)
	}

	request.Salt, err = ethutil.ParseSalt(saltParam)
	if err != nil {
		return nil, fmt.Errorf("error on parameter salt: %w", err)
	}

	request.EpochLength = epochLengthParam
	request.Verbose = verboseParam
	return request, nil
}

func buildApplicationOnlyDeploymentWithoutConsensus(
	ctx context.Context,
	cmd *cobra.Command,
	args []string,
	client *ethclient.Client,
	txOpts *bind.TransactOpts,
) (
	*ethutil.ApplicationDeployment,
	error,
) {
	request := &ethutil.ApplicationDeployment{}
	var err error

	if !cmd.Flags().Changed("factory") {
		request.FactoryAddress, err = config.GetContractsApplicationFactoryAddress()
	} else {
		request.FactoryAddress, err = parseHexAddress(factoryAddressParam)
	}
	if err != nil {
		return nil, fmt.Errorf("error on parameter factory: %w", err)
	}

	if !cmd.Flags().Changed("template-hash") {
		if len(args) >= 2 { // args[1] is mandatory if `template-hash` was absent
			request.TemplateHash, err = readHash(args[1])
		} else {
			err = fmt.Errorf("missing argument. One of `template-path` or `template-hash` is required")
		}
	} else {
		request.TemplateHash, err = parseHexHash(applicationTemplateHashParam)
	}
	if err != nil {
		return nil, fmt.Errorf("error on parameter template-hash: %w", err)
	}

	if !cmd.Flags().Changed("application-owner") {
		request.OwnerAddress = txOpts.From
	} else {
		request.OwnerAddress, err = parseHexAddress(applicationOwnerAddressParam)
	}
	if err != nil {
		return nil, fmt.Errorf("error on parameter application-owner: %w", err)
	}

	if !cmd.Flags().Changed("data-availability") {
		inputBoxAddress := common.Address{}
		inputBoxAddress, err = config.GetContractsInputBoxAddress()
		if err != nil {
			return nil, fmt.Errorf("error on parameter data-availability: %w", err)
		}
		request.InputBoxAddress, request.IInputBoxBlock, request.DataAvailability, err =
			ethutil.DefaultDA(client, inputBoxAddress)
	} else {
		request.InputBoxAddress, request.IInputBoxBlock, request.DataAvailability, err =
			ethutil.CustomDA(client, applicationDataAvailabilityParam)
	}
	if err != nil {
		return nil, fmt.Errorf("error on parameter data-availability: %w", err)
	}

	// ensure there is a contract deployed at the input box address
	code, err := client.CodeAt(ctx, request.InputBoxAddress, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to probe input box address for contract: %v\n", err)
	}
	if len(code) == 0 {
		return nil, fmt.Errorf("error input box address has no code: %v", request.InputBoxAddress)
	}

	request.Salt, err = ethutil.ParseSalt(saltParam)
	if err != nil {
		return nil, fmt.Errorf("error on parameter salt: %w", err)
	}

	request.Verbose = verboseParam
	return request, nil
}

func buildApplicationOnlyDeployment(
	ctx context.Context,
	cmd *cobra.Command,
	args []string,
	client *ethclient.Client,
	txOpts *bind.TransactOpts,
) (
	*ethutil.ApplicationDeployment,
	error,
) {
	request, err := buildApplicationOnlyDeploymentWithoutConsensus(ctx, cmd, args, client, txOpts)

	request.Consensus, err = parseHexAddress(applicationConsensusAddressParam)
	if err != nil {
		return nil, fmt.Errorf("error on parameter consensus: %w", err)
	}

	request.Consensus, request.EpochLength, err = customConsensus(client, applicationConsensusAddressParam)
	if err != nil {
		return nil, fmt.Errorf("error on parameter consensus: %w", err)
	}

	return request, nil
}

func buildPrtApplicationDeployment(
	ctx context.Context,
	cmd *cobra.Command,
	args []string,
	client *ethclient.Client,
	txOpts *bind.TransactOpts,
) (
	*ethutil.PRTApplicationDeployment,
	error,
) {
	var err error
	request := &ethutil.PRTApplicationDeployment{}
	if !cmd.Flags().Changed("prt-factory") {
		request.FactoryAddress, err = config.GetContractsPrtConsensusFactoryAddress()
	} else {
		request.FactoryAddress, err = parseHexAddress(factoryAddressParam)
	}
	if err != nil {
		return nil, fmt.Errorf("error on parameter factory: %w", err)
	}

	request.Application, err = buildApplicationOnlyDeploymentWithoutConsensus(ctx, cmd, args, client, txOpts)
	if err != nil {
		return nil, fmt.Errorf("error on application: %w", err)
	}

	return request, nil
}

// read the hash value from the cartesi machine hash file
func readHash(machineDir string) (common.Hash, error) {
	zero := common.Hash{}
	path := path.Join(machineDir, "hash")
	hash, err := os.ReadFile(path)
	if err != nil {
		return zero, fmt.Errorf("read hash: %w", err)
	} else if len(hash) != common.HashLength {
		return zero, fmt.Errorf(
			"read hash: wrong size; expected %v bytes but read %v",
			common.HashLength,
			len(hash),
		)
	}
	return common.BytesToHash(hash), nil
}

func parseHexHash(hash string) (common.Hash, error) {
	out := common.Hash{}
	return out, out.UnmarshalText([]byte(hash))
}

func customConsensus(client *ethclient.Client, consensusString string) (common.Address, uint64, error) {
	consensusAddress, err := parseHexAddress(consensusString)
	if err != nil {
		return common.Address{}, 0, err
	}

	consensus, err := iconsensus.NewIConsensus(consensusAddress, client)
	if err != nil {
		return common.Address{}, 0, err
	}

	epochLengthBig, err := consensus.GetEpochLength(nil)
	if err != nil {
		return common.Address{}, 0, fmt.Errorf("failed to retrieve consensus epoch length: %v", err)
	}

	return consensusAddress, epochLengthBig.Uint64(), nil
}
