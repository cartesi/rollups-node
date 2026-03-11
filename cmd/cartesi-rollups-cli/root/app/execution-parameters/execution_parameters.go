// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package execution

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cartesi/rollups-node/internal/config"
	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository/factory"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "execution-parameters",
	Short:   "Manages application execution parameters",
	Example: examples,
	Run:     run,
}

const examples = `# Read execution parameters
cartesi-rollups-cli app execution-parameters get echo-dapp parameter
# Set execution parameters
cartesi-rollups-cli app execution-parameters set echo-dapp parameter value
# List
cartesi-rollups-cli app execution-parameters list echo-dapp
# Save
cartesi-rollups-cli app execution-parameters dump echo-dapp > echo-dapp-params.json
# Load
cartesi-rollups-cli app execution-parameters load echo-dapp < echo-dapp-params.json

Note: Duration values can be set using time suffixes (e.g., "11s", "1m", "1h", or "1h20m0.5s").
      When using 'dump' and 'load', durations are represented in nanoseconds.

      Snapshot policy is one of: NONE, EVERY_INPUT, EVERY_EPOCH`

const maxJSONSize = 1 << 20 // 1MB limit
const maxParamLength = 100
const maxValueLength = 100

func setHelpFunc(cmd *cobra.Command) {
	origHelpFunc := cmd.HelpFunc()
	newHelpFunc := func(command *cobra.Command, strings []string) {
		command.Flags().Lookup("verbose").Hidden = false
		command.Flags().Lookup("database-connection").Hidden = false
		origHelpFunc(command, strings)
	}
	cmd.SetHelpFunc(newHelpFunc)
}

func init() {
	// Add subcommands
	Cmd.AddCommand(getCmd)
	Cmd.AddCommand(setCmd)
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(dumpCmd)
	Cmd.AddCommand(loadCmd)

	setHelpFunc(getCmd)
	setHelpFunc(setCmd)
	setHelpFunc(listCmd)
	setHelpFunc(dumpCmd)
	setHelpFunc(loadCmd)

}

func run(cmd *cobra.Command, args []string) {
	// If no subcommand is provided, show help
	err := cmd.Help()
	cobra.CheckErr(err)
}

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get [application] [parameter]",
	Short: "Get a specific configuration parameter",
	Args:  cobra.ExactArgs(2), // nolint: mnd
	Run:   runGet,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

// setCmd represents the set command
var setCmd = &cobra.Command{
	Use:   "set [application] [parameter] [value]",
	Short: "Set a specific configuration parameter",
	Args:  cobra.ExactArgs(3), // nolint: mnd
	Run:   runSet,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list [application]",
	Short: "List all configuration parameters",
	Args:  cobra.ExactArgs(1),
	Run:   runList,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

// dumpCmd represents the list command
var dumpCmd = &cobra.Command{
	Use:   "dump [application]",
	Short: "Dump execution parameters as a JSON object",
	Args:  cobra.ExactArgs(1),
	Run:   runDump,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

// loadCmd represents the load command
var loadCmd = &cobra.Command{
	Use:   "load [application]",
	Short: "Load configuration from stdin",
	Args:  cobra.ExactArgs(1),
	Run:   runLoad,
	Long: `
Supported Environment Variables:
  CARTESI_DATABASE_CONNECTION                    Database connection string`,
}

func runGet(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	cobra.CheckErr(err)

	parameter := strings.TrimSpace(args[1])

	// Validate parameter name length
	if len(parameter) > maxParamLength {
		cobra.CheckErr(fmt.Errorf("parameter name exceeds maximum allowed length of %d characters", maxParamLength))
	}

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	cobra.CheckErr(err)
	defer repo.Close()

	app, err := repo.GetApplication(ctx, nameOrAddress)
	cobra.CheckErr(err)
	if app == nil {
		fmt.Fprintf(os.Stderr, "application %q not found\n", nameOrAddress)
		os.Exit(1)
	}

	params, err := repo.GetExecutionParameters(ctx, app.ID)
	cobra.CheckErr(err)

	value, err := getParameterValue(params, parameter)
	cobra.CheckErr(err)

	fmt.Println(value)
}

func runSet(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	cobra.CheckErr(err)

	parameter := strings.TrimSpace(args[1])
	value := strings.TrimSpace(args[2])

	// Validate parameter name length
	if len(parameter) > maxParamLength {
		cobra.CheckErr(fmt.Errorf("parameter name exceeds maximum allowed length of %d characters", maxParamLength))
	}

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	cobra.CheckErr(err)
	defer repo.Close()

	app, err := repo.GetApplication(ctx, nameOrAddress)
	cobra.CheckErr(err)
	if app == nil {
		fmt.Fprintf(os.Stderr, "application %q not found\n", nameOrAddress)
		os.Exit(1)
	}

	params, err := repo.GetExecutionParameters(ctx, app.ID)
	cobra.CheckErr(err)

	err = setParameterValue(params, parameter, value)
	cobra.CheckErr(err)

	err = params.Validate()
	cobra.CheckErr(err)

	params.UpdatedAt = time.Now()
	err = repo.UpdateExecutionParameters(ctx, params)
	cobra.CheckErr(err)

	fmt.Printf("Parameter %s updated successfully for %s\n", parameter, app.Name)
}

func runList(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	cobra.CheckErr(err)

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	cobra.CheckErr(err)
	defer repo.Close()

	app, err := repo.GetApplication(ctx, nameOrAddress)
	cobra.CheckErr(err)
	if app == nil {
		fmt.Fprintf(os.Stderr, "application %q not found\n", nameOrAddress)
		os.Exit(1)
	}

	params, err := repo.GetExecutionParameters(ctx, app.ID)
	cobra.CheckErr(err)

	printParameters(params)
}

func runDump(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	cobra.CheckErr(err)

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	cobra.CheckErr(err)
	defer repo.Close()

	app, err := repo.GetApplication(ctx, nameOrAddress)
	cobra.CheckErr(err)
	if app == nil {
		fmt.Fprintf(os.Stderr, "application %q not found\n", nameOrAddress)
		os.Exit(1)
	}

	params, err := repo.GetExecutionParameters(ctx, app.ID)
	cobra.CheckErr(err)

	jsonData, err := json.MarshalIndent(params, "", "  ")
	cobra.CheckErr(err)
	fmt.Println(string(jsonData))
}

func runLoad(cmd *cobra.Command, args []string) {
	ctx := cmd.Context()

	nameOrAddress, err := config.ToApplicationNameOrAddressFromString(args[0])
	cobra.CheckErr(err)

	dsn, err := config.GetDatabaseConnection()
	cobra.CheckErr(err)

	repo, err := factory.NewRepositoryFromConnectionString(ctx, dsn.Raw())
	cobra.CheckErr(err)
	defer repo.Close()

	app, err := repo.GetApplication(ctx, nameOrAddress)
	cobra.CheckErr(err)
	if app == nil {
		fmt.Fprintf(os.Stderr, "application %q not found\n", nameOrAddress)
		os.Exit(1)
	}

	// Read JSON from stdin with size limit to prevent memory exhaustion
	lr := &io.LimitedReader{
		R: os.Stdin,
		N: maxJSONSize,
	}
	data, err := io.ReadAll(lr)
	if err != nil {
		cobra.CheckErr(err)
	}
	if lr.N == 0 {
		cobra.CheckErr(fmt.Errorf("input exceeds maximum allowed size of %d bytes", maxJSONSize))
	}

	var params model.ExecutionParameters
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields() // Prevent unexpected fields
	err = decoder.Decode(&params)
	cobra.CheckErr(err)

	// Validate the loaded parameters
	if err := params.Validate(); err != nil {
		cobra.CheckErr(err)
	}

	// Set the application ID
	params.ApplicationID = app.ID

	err = repo.UpdateExecutionParameters(ctx, &params)
	cobra.CheckErr(err)

	fmt.Printf("Configuration loaded successfully for %s\n", app.Name)
}

func getParameterValue(params *model.ExecutionParameters, parameter string) (string, error) {
	parameter = strings.ToLower(parameter)
	switch parameter {
	case "snapshot_policy":
		return string(params.SnapshotPolicy), nil
	case "advance_inc_cycles":
		return fmt.Sprintf("%d", params.AdvanceIncCycles), nil
	case "advance_max_cycles":
		return fmt.Sprintf("%d", params.AdvanceMaxCycles), nil
	case "inspect_inc_cycles":
		return fmt.Sprintf("%d", params.InspectIncCycles), nil
	case "inspect_max_cycles":
		return fmt.Sprintf("%d", params.InspectMaxCycles), nil
	case "advance_inc_deadline":
		return params.AdvanceIncDeadline.String(), nil
	case "advance_max_deadline":
		return params.AdvanceMaxDeadline.String(), nil
	case "inspect_inc_deadline":
		return params.InspectIncDeadline.String(), nil
	case "inspect_max_deadline":
		return params.InspectMaxDeadline.String(), nil
	case "load_deadline":
		return params.LoadDeadline.String(), nil
	case "store_deadline":
		return params.StoreDeadline.String(), nil
	case "fast_deadline":
		return params.FastDeadline.String(), nil
	case "max_concurrent_inspects":
		return fmt.Sprintf("%d", params.MaxConcurrentInspects), nil
	default:
		return "", fmt.Errorf("unknown parameter: %s", parameter)
	}
}

func setParameterValue(params *model.ExecutionParameters, parameter, value string) error {
	// Limit input size to prevent potential DoS
	if len(value) > maxValueLength {
		return fmt.Errorf("parameter value exceeds maximum allowed length of %d characters", maxValueLength)
	}

	parameter = strings.ToLower(parameter)
	switch parameter {
	case "snapshot_policy":
		value = strings.ToUpper(value)
		switch model.SnapshotPolicy(value) {
		case model.SnapshotPolicy_None, model.SnapshotPolicy_EveryInput, model.SnapshotPolicy_EveryEpoch:
			params.SnapshotPolicy = model.SnapshotPolicy(value)
		default:
			return fmt.Errorf("invalid snapshot policy: %s. Valid values are: NONE, EVERY_INPUT, EVERY_EPOCH", value)
		}
	case "advance_inc_cycles":
		val, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid value for advance_inc_cycles: %w", err)
		}
		params.AdvanceIncCycles = val
	case "advance_max_cycles":
		val, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid value for advance_max_cycles: %w", err)
		}
		params.AdvanceMaxCycles = val
	case "inspect_inc_cycles":
		val, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid value for inspect_inc_cycles: %w", err)
		}
		params.InspectIncCycles = val
	case "inspect_max_cycles":
		val, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid value for inspect_max_cycles: %w", err)
		}
		params.InspectMaxCycles = val
	case "advance_inc_deadline":
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid value for advance_inc_deadline: %w", err)
		}
		params.AdvanceIncDeadline = duration
	case "advance_max_deadline":
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid value for advance_max_deadline: %w", err)
		}
		params.AdvanceMaxDeadline = duration
	case "inspect_inc_deadline":
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid value for inspect_inc_deadline: %w", err)
		}
		params.InspectIncDeadline = duration
	case "inspect_max_deadline":
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid value for inspect_max_deadline: %w", err)
		}
		params.InspectMaxDeadline = duration
	case "load_deadline":
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid value for load_deadline: %w", err)
		}
		params.LoadDeadline = duration
	case "store_deadline":
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid value for store_deadline: %w", err)
		}
		params.StoreDeadline = duration
	case "fast_deadline":
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid value for fast_deadline: %w", err)
		}
		params.FastDeadline = duration
	case "max_concurrent_inspects":
		val, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid value for max_concurrent_inspects: %w", err)
		}
		params.MaxConcurrentInspects = uint32(val)
	default:
		return fmt.Errorf("unknown parameter: %s", parameter)
	}
	return nil
}

func printParameters(params *model.ExecutionParameters) {
	fmt.Printf("snapshot_policy: %s\n", params.SnapshotPolicy)
	fmt.Printf("advance_inc_cycles: %d\n", params.AdvanceIncCycles)
	fmt.Printf("advance_max_cycles: %d\n", params.AdvanceMaxCycles)
	fmt.Printf("inspect_inc_cycles: %d\n", params.InspectIncCycles)
	fmt.Printf("inspect_max_cycles: %d\n", params.InspectMaxCycles)
	fmt.Printf("advance_inc_deadline: %s\n", params.AdvanceIncDeadline)
	fmt.Printf("advance_max_deadline: %s\n", params.AdvanceMaxDeadline)
	fmt.Printf("inspect_inc_deadline: %s\n", params.InspectIncDeadline)
	fmt.Printf("inspect_max_deadline: %s\n", params.InspectMaxDeadline)
	fmt.Printf("load_deadline: %s\n", params.LoadDeadline)
	fmt.Printf("store_deadline: %s\n", params.StoreDeadline)
	fmt.Printf("fast_deadline: %s\n", params.FastDeadline)
	fmt.Printf("max_concurrent_inspects: %d\n", params.MaxConcurrentInspects)
}
