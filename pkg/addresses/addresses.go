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
	CartesiDAppFactory  common.Address
	DAppAddressRelay    common.Address
	ERC1155BatchPortal  common.Address
	ERC1155SinglePortal common.Address
	ERC20Portal         common.Address
	ERC721Portal        common.Address
	EtherPortal         common.Address
	InputBox            common.Address
	CartesiDApp         common.Address
}

// Get the addresses for the test environment.
func GetTestBook() *Book {
	return &Book{
		CartesiDAppFactory:  common.HexToAddress("0x7122cd1221C20892234186facfE8615e6743Ab02"),
		DAppAddressRelay:    common.HexToAddress("0xF5DE34d6BbC0446E2a45719E718efEbaaE179daE"),
		ERC1155BatchPortal:  common.HexToAddress("0xedB53860A6B52bbb7561Ad596416ee9965B055Aa"),
		ERC1155SinglePortal: common.HexToAddress("0x7CFB0193Ca87eB6e48056885E026552c3A941FC4"),
		ERC20Portal:         common.HexToAddress("0x9C21AEb2093C32DDbC53eEF24B873BDCd1aDa1DB"),
		ERC721Portal:        common.HexToAddress("0x237F8DD094C0e47f4236f12b4Fa01d6Dae89fb87"),
		EtherPortal:         common.HexToAddress("0xFfdbe43d4c855BF7e0f105c400A50857f53AB044"),
		InputBox:            common.HexToAddress("0x59b22D57D4f067708AB0c00552767405926dc768"),
		CartesiDApp:         common.HexToAddress("0x70ac08179605AF2D9e75782b8DEcDD3c22aA4D0C"),
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
