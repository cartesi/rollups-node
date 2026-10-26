// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package prt

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// staticABIWordCount returns the exact static data size in ABI words.
func staticABIWordCount(arguments abi.Arguments) (int, error) {
	words := 0
	for _, argument := range arguments {
		size, err := staticABITypeWordCount(argument.Type)
		if err != nil {
			return 0, err
		}
		words += size
	}
	return words, nil
}

func staticABITypeWordCount(value abi.Type) (int, error) {
	switch value.T {
	case abi.ArrayTy:
		words, err := staticABITypeWordCount(*value.Elem)
		return value.Size * words, err
	case abi.TupleTy:
		words := 0
		for _, element := range value.TupleElems {
			size, err := staticABITypeWordCount(*element)
			if err != nil {
				return 0, err
			}
			words += size
		}
		return words, nil
	case abi.IntTy, abi.UintTy, abi.BoolTy, abi.AddressTy, abi.FixedBytesTy, abi.HashTy, abi.FixedPointTy, abi.FunctionTy:
		return 1, nil
	default:
		return 0, fmt.Errorf("ABI type %s is not static", value.String())
	}
}
