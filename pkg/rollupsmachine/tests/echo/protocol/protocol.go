// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package protocol

import (
	"encoding/json"
	"math/big"
)

var (
	VoucherValue *big.Int = big.NewInt(115)

	AdvanceException = []byte("big bad output exception")
	InspectException = []byte("big bad report exception")
)

type Data interface {
	ToBytes() ([]byte, error)
}

type InputData struct {
	Quote     string `json:"quote"`
	Vouchers  int    `json:"vouchers"`
	Notices   int    `json:"notices"`
	Reports   int    `json:"reports"`
	Reject    bool   `json:"reject"`
	Exception bool   `json:"exception"`
}

type QueryData struct {
	Quote     string `json:"quote"`
	Reports   int    `json:"reports"`
	Reject    bool   `json:"reject"`
	Exception bool   `json:"exception"`
}

type VoucherData struct {
	Counter int    `json:"counter"`
	Quote   string `json:"quote"`
	Index   int    `json:"index"`
}

type NoticeData struct {
	Counter int    `json:"counter"`
	Quote   string `json:"quote"`
	Index   int    `json:"index"`
}

type Report struct {
	Counter int    `json:"counter"`
	Quote   string `json:"quote"`
	Index   int    `json:"index"`
}

func FromBytes[T Data](bytes []byte) (data T) {
	err := json.Unmarshal(bytes, &data)
	if err != nil {
		panic(err)
	}
	return
}

func toBytes(data any) ([]byte, error) { return json.MarshalIndent(data, "", "\t") }

func (data InputData) ToBytes() ([]byte, error)   { return toBytes(data) }
func (data QueryData) ToBytes() ([]byte, error)   { return toBytes(data) }
func (data VoucherData) ToBytes() ([]byte, error) { return toBytes(data) }
func (data NoticeData) ToBytes() ([]byte, error)  { return toBytes(data) }
func (data Report) ToBytes() ([]byte, error)      { return toBytes(data) }
