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
	Application         common.Address
	ApplicationFactory  common.Address
	Authority           common.Address
	AuthorityFactory    common.Address
	DAppAddressRelay    common.Address
	ERC1155BatchPortal  common.Address
	ERC1155SinglePortal common.Address
	ERC20Portal         common.Address
	ERC721Portal        common.Address
	EtherPortal         common.Address
	InputBox            common.Address
}

// Get the addresses for the test environment.
func GetTestBook() *Book {
	return &Book{
		Application:         common.HexToAddress("0xb72c832dDeA10326143831F1E5F1646920C9c990"),
		ApplicationFactory:  common.HexToAddress("0x39cc8d1faB70F713784032f166aB7Fe3B4801144"),
		Authority:           common.HexToAddress("0x77e5a5fb18F72b5106621f66C704c006c6dB4578"),
		AuthorityFactory:    common.HexToAddress("0x5EF4260c72a7A8df752AFF49aC46Ba741754E04a"),
		ERC1155BatchPortal:  common.HexToAddress("0x83D7fc8A2A2535A17b037598bad23562215a752A"),
		ERC1155SinglePortal: common.HexToAddress("0x77b5b758f43E789E0858a766934bE08B2CD65feA"),
		ERC20Portal:         common.HexToAddress("0x8f4b3F53699EDd5374c3374b4Ee1CcA3d23E95Ab"),
		ERC721Portal:        common.HexToAddress("0xDF9d6F65E9a053FbaFF9eAaf0b522f1b35Dfd05B"),
		EtherPortal:         common.HexToAddress("0xF03FB966604bF02073b87b4586b3edBC201f73A6"),
		InputBox:            common.HexToAddress("0xA1b8EB1F13d8D5Db976a653BbDF8972cfD14691C"),
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
