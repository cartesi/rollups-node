// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package savesnapshot

import (
	"github.com/cartesi/rollups-node/internal/machine"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:     "save-snapshot",
	Short:   "Saves the testing Cartesi machine snapshot to the designated folder",
	Example: examples,
	Run:     run,
}

const examples = `# Save the default Rollups Echo Application snapshot:
cartesi-rollups-cli save-snapshot`

var (
	sourceDockerImage string
	tempContainerName string
	destDir           string
)

func init() {
	Cmd.Flags().StringVar(&sourceDockerImage, "docker-image",
		"cartesi/rollups-node-snapshot:devel",
		"Docker image containing the Cartesi Machine snapshot to be used")

	Cmd.Flags().StringVar(&tempContainerName, "temp-container-name", "temp-machine",
		"Name of the temporary container needed to extract the machine snapshot files")

	Cmd.Flags().StringVar(&destDir, "dest-dir", "./machine-snapshot",
		"directory where to store the Cartesi Machine snapshot to be used by the local Node")
}

func run(cmd *cobra.Command, args []string) {

	err := machine.Save(sourceDockerImage, destDir, tempContainerName)

	cobra.CheckErr(err)
}
