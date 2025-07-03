// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This binary generates the Go bindings for the Cartesi Rollups contracts.
// This binary should be called with `go generate` in the parent dir.
// First, it downloads the Cartesi Rollups npm package containing the contracts.
// Then, it generates the bindings using abi-gen.
// Finally, it stores the bindings in the current directory.
package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
)

const rollupsContractsPath = "../../rollups-contracts/"
const rollupsPrtContractsPath = "../../rollups-prt-contracts/"

type contractBinding struct {
	jsonPath string
	typeName string
}

var bindings = []contractBinding{
	{
		jsonPath: rollupsContractsPath + "IAuthorityFactory.sol/IAuthorityFactory.json",
		typeName: "IAuthorityFactory",
	},
	{
		jsonPath: rollupsContractsPath + "IConsensus.sol/IConsensus.json",
		typeName: "IConsensus",
	},
	{
		jsonPath: rollupsContractsPath + "IApplication.sol/IApplication.json",
		typeName: "IApplication",
	},
	{
		jsonPath: rollupsContractsPath + "IApplicationFactory.sol/IApplicationFactory.json",
		typeName: "IApplicationFactory",
	},
	{
		jsonPath: rollupsContractsPath + "ISelfHostedApplicationFactory.sol/ISelfHostedApplicationFactory.json",
		typeName: "ISelfHostedApplicationFactory",
	},
	{
		jsonPath: rollupsContractsPath + "IInputBox.sol/IInputBox.json",
		typeName: "IInputBox",
	},
	{
		jsonPath: rollupsContractsPath + "Inputs.sol/Inputs.json",
		typeName: "Inputs",
	},
	{
		jsonPath: rollupsContractsPath + "Outputs.sol/Outputs.json",
		typeName: "Outputs",
	},
	{
		jsonPath: rollupsContractsPath + "DataAvailability.sol/DataAvailability.json",
		typeName: "DataAvailability",
	},
	{
		jsonPath: rollupsPrtContractsPath + "prt/contracts/out/Tournament.sol/Tournament.json",
		typeName: "Tournament",
	},
	{
		jsonPath: rollupsPrtContractsPath + "prt/contracts/out/LeafTournament.sol/LeafTournament.json",
		typeName: "LeafTournament",
	},
	{
		jsonPath: rollupsPrtContractsPath + "prt/contracts/out/NonLeafTournament.sol/NonLeafTournament.json",
		typeName: "NonLeafTournament",
	},
	{
		jsonPath: rollupsPrtContractsPath + "prt/contracts/out/RootTournament.sol/RootTournament.json",
		typeName: "RootTournament",
	},
	{
		jsonPath: rollupsPrtContractsPath + "prt/contracts/out/NonRootTournament.sol/NonRootTournament.json",
		typeName: "NonRootTournament",
	},
	{
		jsonPath: rollupsPrtContractsPath + "prt/contracts/out/IMultiLevelTournamentFactory.sol/IMultiLevelTournamentFactory.json",
		typeName: "IMultiLevelTournamentFactory",
	},
	{
		jsonPath: rollupsPrtContractsPath + "cartesi-rollups/contracts/out/DaveConsensus.sol/DaveConsensus.json",
		typeName: "DaveConsensus",
	},
	{
		jsonPath: rollupsPrtContractsPath + "cartesi-rollups/contracts/out/DaveConsensusFactory.sol/DaveConsensusFactory.json",
		typeName: "DaveConsensusFactory",
	},
}

func main() {
	files := make(map[string]bool)
	for _, b := range bindings {
		files[b.jsonPath] = true
	}
	contents := readFilesFromDir(files)

	for _, b := range bindings {
		content := contents[b.jsonPath]
		if content == nil {
			log.Fatal("missing contents for ", b.jsonPath)
		}
		generateBinding(b, content)
	}
}

// Exit if there is any error.
func checkErr(context string, err any) {
	if err != nil {
		log.Fatal(context, ": ", err)
	}
}

// Read the required files from the directory.
// Return a map with the file contents.
func readFilesFromDir(files map[string]bool) map[string][]byte {
	contents := make(map[string][]byte)
	for fileName := range files {
		fileFullPath, err := filepath.Abs(fileName)
		if err != nil {
			log.Fatal(err)
		}
		data, err := os.ReadFile(fileFullPath)
		checkErr("read file", err)
		contents[fileName] = data
	}
	return contents
}

// Get the .abi key from the json
func getAbi(rawJson []byte) []byte {
	var contents struct {
		Abi json.RawMessage `json:"abi"`
	}
	err := json.Unmarshal(rawJson, &contents)
	checkErr("decode json", err)
	return contents.Abi
}

// Check whether file exists.
func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !errors.Is(err, fs.ErrNotExist)
}

// Generate the Go bindings for the contracts.
func generateBinding(b contractBinding, content []byte) {
	var (
		pkg     = strings.ToLower(b.typeName)
		sigs    []map[string]string
		abis    = []string{string(getAbi(content))}
		bins    = []string{""}
		types   = []string{b.typeName}
		libs    = make(map[string]string)
		aliases = make(map[string]string)
	)
	code, err := bind.Bind(types, abis, bins, sigs, pkg, libs, aliases)
	checkErr("generate binding", err)

	if fileExists(pkg) {
		err := os.RemoveAll(pkg)
		checkErr("removing dir", err)
	}

	const dirMode = 0700
	err = os.Mkdir(pkg, dirMode)
	checkErr("creating dir", err)

	const fileMode = 0600
	filePath := pkg + "/" + pkg + ".go"
	err = os.WriteFile(filePath, []byte(code), fileMode)
	checkErr("write binding file", err)

	log.Print("generated binding for ", filePath)
}
