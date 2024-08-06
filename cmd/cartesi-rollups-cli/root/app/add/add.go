// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package add

import (
	"fmt"
	"log/slog"
	"os"

	cmdcommom "github.com/cartesi/rollups-node/cmd/cartesi-rollups-cli/root/common"
	"github.com/cartesi/rollups-node/internal/node/model"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "add",
	Short:   "Adds a new application",
	Example: examples,
	Run:     run,
}

const examples = `# Adds an application to Rollups Node:
cartesi-rollups-cli app add -a 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF -n 10`

const (
	statusRunning    = "running"
	statusNotRunning = "not-running"
)

var (
	applicationAddress            string
	templateHash                  string
	inputBoxDeploymentBlockNumber uint64
	snapshotUri                   string
	status                        string
)

func init() {

	Cmd.Flags().StringVarP(
		&applicationAddress,
		"address",
		"a",
		"",
		"Application contract address",
	)

	Cmd.Flags().StringVarP(
		&templateHash,
		"template-hash",
		"t",
		"",
		"Application template hash",
	)

	Cmd.Flags().Uint64VarP(
		&inputBoxDeploymentBlockNumber,
		"inputbox-block-number",
		"n",
		0,
		"InputBox deployment block number",
	)

	Cmd.Flags().StringVarP(
		&snapshotUri,
		"snapshot-uri",
		"u",
		"",
		"Application snapshot URI",
	)

	Cmd.Flags().StringVarP(
		&status,
		"status",
		"s",
		statusRunning,
		"Sets the application status",
	)

	cobra.CheckErr(Cmd.MarkFlagRequired("address"))
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

	application := model.Application{
		ContractAddress:    common.HexToAddress(applicationAddress),
		TemplateHash:       common.HexToHash(templateHash),
		LastProcessedBlock: inputBoxDeploymentBlockNumber,
		Status:             applicationStatus,
	}

	err := cmdcommom.Database.InsertApplication(ctx, &application)
	cobra.CheckErr(err)
	fmt.Printf("Application %v successfully added\n", application.ContractAddress)
}
