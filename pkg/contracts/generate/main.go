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

const baseContractsPath = "../../rollups-contracts/export/artifacts/contracts/"

type contractBinding struct {
	jsonPath string
	typeName string
}

var bindings = []contractBinding{
	{
		jsonPath: baseContractsPath + "consensus/authority/IAuthorityFactory.sol/IAuthorityFactory.json",
		typeName: "IAuthorityFactory",
	},
	{
		jsonPath: baseContractsPath + "consensus/IConsensus.sol/IConsensus.json",
		typeName: "IConsensus",
	},
	{
		jsonPath: baseContractsPath + "dapp/IApplication.sol/IApplication.json",
		typeName: "IApplication",
	},
	{
		jsonPath: baseContractsPath + "dapp/IApplicationFactory.sol/IApplicationFactory.json",
		typeName: "IApplicationFactory",
	},
	{
		jsonPath: baseContractsPath + "dapp/ISelfHostedApplicationFactory.sol/ISelfHostedApplicationFactory.json",
		typeName: "ISelfHostedApplicationFactory",
	},
	{
		jsonPath: baseContractsPath + "inputs/IInputBox.sol/IInputBox.json",
		typeName: "IInputBox",
	},
	{
		jsonPath: baseContractsPath + "common/Inputs.sol/Inputs.json",
		typeName: "Inputs",
	},
	{
		jsonPath: baseContractsPath + "common/Outputs.sol/Outputs.json",
		typeName: "Outputs",
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
	code, err := bind.Bind(types, abis, bins, sigs, pkg, bind.LangGo, libs, aliases)
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
