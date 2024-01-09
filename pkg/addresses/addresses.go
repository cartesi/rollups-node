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
	AuthorityHistoryPairFactory common.Address
	CartesiDAppFactory          common.Address
	DAppAddressRelay            common.Address
	ERC1155BatchPortal          common.Address
	ERC1155SinglePortal         common.Address
	ERC20Portal                 common.Address
	ERC721Portal                common.Address
	EtherPortal                 common.Address
	InputBox                    common.Address
	CartesiDApp                 common.Address
}

// Get the addresses for the test environment.
func GetTestBook() *Book {
	return &Book{
		AuthorityHistoryPairFactory: common.
			HexToAddress("0x3890A047Cf9Af60731E80B2105362BbDCD70142D"),
		CartesiDAppFactory:  common.HexToAddress("0x610178dA211FEF7D417bC0e6FeD39F05609AD788"),
		DAppAddressRelay:    common.HexToAddress("0x8A791620dd6260079BF849Dc5567aDC3F2FdC318"),
		ERC1155BatchPortal:  common.HexToAddress("0x0165878A594ca255338adfa4d48449f69242Eb8F"),
		ERC1155SinglePortal: common.HexToAddress("0x2279B7A0a67DB372996a5FaB50D91eAA73d2eBe6"),
		ERC20Portal:         common.HexToAddress("0xa513E6E4b8f2a923D98304ec87F64353C4D5C853"),
		ERC721Portal:        common.HexToAddress("0xDc64a140Aa3E981100a9becA4E685f962f0cF6C9"),
		EtherPortal:         common.HexToAddress("0x5FC8d32690cc91D4c39d9d3abcBD16989F875707"),
		InputBox:            common.HexToAddress("0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9"),
		CartesiDApp:         common.HexToAddress("0x180763470853cAF642Df79a908F9282c61692A45"),
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
