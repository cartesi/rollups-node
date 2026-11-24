// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package postgres

import (
	"testing"
	"unsafe"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/ethereum/go-ethereum/common"
)

func TestStructByteSize(t *testing.T) {
	hash := common.Hash{}
	out := model.Output{
		RawData:              make([]byte, 3, 8),
		Hash:                 &hash,
		OutputHashesSiblings: []common.Hash{{}, {}},
	}
	want := uint64(unsafe.Sizeof(out) + 3 + 3*unsafe.Sizeof(hash))
	if got := structByteSize(out); got != want {
		t.Errorf("output size = %d, want %d", got, want)
	}

	uri := "snapshot"
	in := model.Input{
		ExceptionData: []byte{1, 2},
		SnapshotURI:   &uri,
	}
	want = uint64(unsafe.Sizeof(in)+unsafe.Sizeof(uri)) + uint64(2+len(uri))
	if got := structByteSize(in); got != want {
		t.Errorf("input size = %d, want %d", got, want)
	}
}

func TestPageByteBudget(t *testing.T) {
	row := model.Report{RawData: []byte{1, 2, 3}}
	size := structByteSize(row)
	budget := pageByteBudget{max: 2 * size}
	if !budget.add(row) || !budget.add(row) {
		t.Fatal("budget rejected a page that fits exactly")
	}
	if budget.add(row) {
		t.Fatal("budget accepted a row past the page limit")
	}

	unlimited := pageByteBudget{}
	if !unlimited.add(row) || !unlimited.add(row) {
		t.Fatal("zero byte limit rejected a row")
	}
}
