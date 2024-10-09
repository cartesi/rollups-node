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
	ChainId                      uint64
	ApplicationFactory           common.Address
	AuthorityFactory             common.Address
	ERC1155BatchPortal           common.Address
	ERC1155SinglePortal          common.Address
	ERC20Portal                  common.Address
	ERC721Portal                 common.Address
	EtherPortal                  common.Address
	InputBox                     common.Address
	QuorumFactory                common.Address
	SafeERC20Transfer            common.Address
	SelfHostedApplicationFactory common.Address
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
