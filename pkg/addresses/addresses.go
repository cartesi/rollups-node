// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// This package manages the contract addresses.
//
// The addresses depend on the deployment of the contracts and should be provided by the node user.
// This module offers an option to load these addresses from a config file, compatible with the
// output of `sunodo address-book --json`.
// This package also contain the addresses for the test environment as hard-coded values.
package addresses

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common"
)

// List of contract addresses.
type Book struct {
	Application                  common.Address
	ApplicationFactory           common.Address
	Authority                    common.Address
	AuthorityFactory             common.Address
	DAppAddressRelay             common.Address
	ERC1155BatchPortal           common.Address
	ERC1155SinglePortal          common.Address
	ERC20Portal                  common.Address
	ERC721Portal                 common.Address
	EtherPortal                  common.Address
	InputBox                     common.Address
	QuorumFactory                common.Address
	SelfHostedApplicationFactory common.Address
	SafeERC20Transfer            common.Address
}

// Get the addresses for the test environment.
func GetTestBook() *Book {
	return &Book{
		Application: common.HexToAddress(
			"0x1b0FAD42f016a9EBa358c7491A67fa1fAE82912A"),
		ApplicationFactory: common.HexToAddress(
			"0xA1DA32BF664109D62208a1cb0d69aACc6a484873"),
		Authority: common.HexToAddress(
			"0x3fd5dc9dCf5Df3c7002C0628Eb9AD3bb5e2ce257"),
		AuthorityFactory: common.HexToAddress(
			"0xbDC5D42771A4Ae55eC7670AAdD2458D1d9C7C8A8"),
		ERC1155BatchPortal: common.HexToAddress(
			"0x4a218D331C0933d7E3EB496ac901669f28D94981"),
		ERC1155SinglePortal: common.HexToAddress(
			"0x2f0D587DD6EcF67d25C558f2e9c3839c579e5e38"),
		ERC20Portal: common.HexToAddress(
			"0xB0e28881FF7ee9CD5B1229d570540d74bce23D39"),
		ERC721Portal: common.HexToAddress(
			"0x874b3245ead7474Cb9f3b83cD1446dC522f6bd36"),
		EtherPortal: common.HexToAddress(
			"0xfa2292f6D85ea4e629B156A4f99219e30D12EE17"),
		InputBox: common.HexToAddress(
			"0x593E5BCf894D6829Dd26D0810DA7F064406aebB6"),
		QuorumFactory: common.HexToAddress(
			"0x68C3d53a095f66A215a8bEe096Cd3Ba4fFB7bAb3"),
		SelfHostedApplicationFactory: common.HexToAddress(
			"0x0678FAA399F0193Fb9212BE41590316D275b1392"),
		SafeERC20Transfer: common.HexToAddress(
			"0x817b126F242B5F184Fa685b4f2F91DC99D8115F9"),
	}
}

// Get the address book from json File.
func GetBookFromFile(path string) (*Book, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read address book file: %v", err)
	}
	var book Book
	err = json.Unmarshal(data, &book)
	if err != nil {
		return nil, fmt.Errorf("parse address book json: %v", err)
	}
	return &book, nil
}
