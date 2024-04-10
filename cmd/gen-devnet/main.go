// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cartesi/rollups-node/internal/services"
	"github.com/spf13/cobra"
)

const (
	CMD_NAME        = "gen-devnet"
	ANVIL_IP_ADDR   = "0.0.0.0"
	ANVIL_HTTP_PORT = "8545"
	RPC_URL         = "http://" + ANVIL_IP_ADDR + ":" + ANVIL_HTTP_PORT
	USER_FILE_MODE  = 0664
)

var Cmd = &cobra.Command{
	Use:   CMD_NAME,
	Short: "Generates a devnet for testing",
	Long: `Generates a devnet to be used for testing.
It uses a previously generated Cartesi Machine snapshot
and deploys an Application based on its hash file.

Set CARTESI_LOG_LEVEL to debug for extensive logging.`,
	Run: run,
}

var (
	VerboseLog           bool
	anvilStatePath       string
	deploymentInfoPath   string
	hashFile             string
	rollupsContractsPath string
)

func init() {
	// Default path based on submodule location for rollups-contracts 2.0
	Cmd.Flags().StringVarP(&rollupsContractsPath,
		"rollups-contracts-hardhat-path",
		"r",
		"rollups-contracts",
		"path for the hardhat project used to deploy rollups-contracts")

	Cmd.Flags().StringVarP(&hashFile,
		"template-hash-file",
		"t",
		"",
		"path for a Cartesi Machine template hash file")
	cobra.CheckErr(Cmd.MarkFlagRequired("template-hash-file"))

	Cmd.Flags().StringVarP(&anvilStatePath,
		"anvil-state-file",
		"a",
		"./anvil_state.json",
		"path for the resulting anvil state file")

	Cmd.Flags().StringVarP(&deploymentInfoPath,
		"deployment-info-file",
		"d",
		"./deployment.json",
		"path for saving the deployment information")

	Cmd.Flags().BoolVarP(&VerboseLog,
		"verbose",
		"v",
		false,
		"enable verbose logging")
}

func main() {
	err := Cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rollupsContractsPath, err := filepath.Abs(rollupsContractsPath)
	if err != nil {
		fmt.Printf("%s: %v does not exist\n", CMD_NAME, err)
		return
	}

	hash, err := readMachineHash(hashFile)
	if err != nil {
		fmt.Printf("%s: failed to read hash: %v\n", CMD_NAME, err)
		return
	}

	// Supervisor handles only Anvil
	ready := make(chan struct{}, 1)
	go func() {
		var s []services.Service

		s = append(s, newAnvilService(anvilStatePath))

		supervisor := newSupervisorService(s)
		if err := supervisor.Start(ctx, ready); err != nil {
			fmt.Printf("%s: %v", supervisor.Name, err)
			cancel()
		}
	}()

	// Deploy rollups-contracts and create Cartesi Application
	go func() {
		defer cancel()

		select {
		case <-ready:
			depInfo, err := deploy(ctx, rollupsContractsPath, hash)
			if err != nil {
				fmt.Printf("%s: deployment failed. %v\n", CMD_NAME, err)
				return
			}

			anvilStatePath, err := filepath.Abs(anvilStatePath)
			if err != nil {
				fmt.Printf("%s: unable to get path for %s: %v\n",
					CMD_NAME,
					anvilStatePath,
					err)
			} else {
				fmt.Printf("%s: anvil state saved to %s\n",
					CMD_NAME,
					anvilStatePath)
			}

			jsonInfo, err := json.MarshalIndent(depInfo, "", "\t")
			if err != nil {
				fmt.Printf("%s: failed to parse deployment info. %v\n", CMD_NAME, err)
			} else {
				deploymentInfoPath, err := filepath.Abs(deploymentInfoPath)
				if err == nil {
					err = os.WriteFile(deploymentInfoPath, []byte(jsonInfo), USER_FILE_MODE)
					if err == nil {
						fmt.Printf("%s: deployment information saved to %s\n",
							CMD_NAME,
							deploymentInfoPath)
					}
				}
				if err != nil {
					fmt.Printf("%s: unable to save deployment information to %s: %v\n",
						CMD_NAME,
						deploymentInfoPath,
						err)
				}
			}

		case <-ctx.Done():
		}
	}()

	<-ctx.Done()
}

// Read template machine hash from file
func readMachineHash(hashPath string) (string, error) {
	data, err := os.ReadFile(hashPath)
	if err != nil {
		return "", fmt.Errorf("error reading %v (%v)", hashPath, err)
	}

	return hex.EncodeToString(data), nil
}

func newSupervisorService(s []services.Service) services.SupervisorService {
	return services.SupervisorService{
		Name:     CMD_NAME + "-supervisor",
		Services: s,
	}
}
