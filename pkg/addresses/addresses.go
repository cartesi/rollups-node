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
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common"
)

// List of contract addresses.
type Book struct {
	AuthorityHistoryPairFactory common.Address `json:"authorityHistoryPairFactory"`
	CartesiDAppFactory          common.Address `json:"cartesiDAppFactory"`
	DAppAddressRelay            common.Address `json:"dappAddressRelay"`
	ERC1155BatchPortal          common.Address `json:"erc1155BatchPortal"`
	ERC1155SinglePortal         common.Address `json:"erc1155SinglePortal"`
	ERC20Portal                 common.Address `json:"erc20Portal"`
	ERC721Portal                common.Address `json:"erc721Portal"`
	EtherPortal                 common.Address `json:"etherPortal"`
	InputBox                    common.Address `json:"inputBox"`
	CartesiDApp                 common.Address `json:"cartesiDApp"`
	HistoryAddress              common.Address `json:"historyAddress"`
	AuthorityAddress            common.Address `json:"authorityAddress"`
}

// validate checks that no address in the book is a zero address.
func (b *Book) validate() error {
	zero := common.Address{}

	fields := map[string]common.Address{
		"AuthorityHistoryPairFactory": b.AuthorityHistoryPairFactory,
		"CartesiDAppFactory":          b.CartesiDAppFactory,
		"DAppAddressRelay":            b.DAppAddressRelay,
		"ERC1155BatchPortal":          b.ERC1155BatchPortal,
		"ERC1155SinglePortal":         b.ERC1155SinglePortal,
		"ERC20Portal":                 b.ERC20Portal,
		"ERC721Portal":                b.ERC721Portal,
		"EtherPortal":                 b.EtherPortal,
		"InputBox":                    b.InputBox,
		"CartesiDApp":                 b.CartesiDApp,
		"HistoryAddress":              b.HistoryAddress,
		"AuthorityAddress":            b.AuthorityAddress,
	}

	for name, addr := range fields {
		if addr == zero {
			return fmt.Errorf("missing or zero address for %s", name)
		}
	}

	return nil
}

// Get the addresses for the test environment.
func GetTestBook() *Book {
	return &Book{
		AuthorityHistoryPairFactory: common.
			HexToAddress("0x3890A047Cf9Af60731E80B2105362BbDCD70142D"),
		CartesiDAppFactory:  common.HexToAddress("0x7122cd1221C20892234186facfE8615e6743Ab02"),
		DAppAddressRelay:    common.HexToAddress("0xF5DE34d6BbC0446E2a45719E718efEbaaE179daE"),
		ERC1155BatchPortal:  common.HexToAddress("0xedB53860A6B52bbb7561Ad596416ee9965B055Aa"),
		ERC1155SinglePortal: common.HexToAddress("0x7CFB0193Ca87eB6e48056885E026552c3A941FC4"),
		ERC20Portal:         common.HexToAddress("0x9C21AEb2093C32DDbC53eEF24B873BDCd1aDa1DB"),
		ERC721Portal:        common.HexToAddress("0x237F8DD094C0e47f4236f12b4Fa01d6Dae89fb87"),
		EtherPortal:         common.HexToAddress("0xFfdbe43d4c855BF7e0f105c400A50857f53AB044"),
		InputBox:            common.HexToAddress("0x59b22D57D4f067708AB0c00552767405926dc768"),
		CartesiDApp:         common.HexToAddress("0x7C54E3f7A8070a54223469965A871fB8f6f88c22"),
		HistoryAddress:      common.HexToAddress("0x325272217ae6815b494bF38cED004c5Eb8a7CdA7"),
		AuthorityAddress:    common.HexToAddress("0x58c93F83fb3304730C95aad2E360cdb88b782010"),
	}
}

// Get the address book from json File.
func GetBookFromFile(path string) (*Book, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read address book file: %v", err)
	}

	var book Book
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&book); err != nil {
		return nil, fmt.Errorf("parse address book json: %v", err)
	}

	if err := book.validate(); err != nil {
		return nil, fmt.Errorf("invalid address book: %v", err)
	}

	return &book, nil
}
