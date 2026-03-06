// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestEpochJSONRoundtrip(t *testing.T) {
	root := common.HexToHash("0xabcd")
	original := Epoch{
		ApplicationID:        1,
		Index:                42,
		FirstBlock:           100,
		LastBlock:            200,
		InputIndexLowerBound: 0,
		InputIndexUpperBound: 10,
		VirtualIndex:         5,
		Status:               EpochStatus_ClaimAccepted,
		OutputsMerkleRoot:    &root,
		CreatedAt:            time.Now().Truncate(time.Microsecond).UTC(),
		UpdatedAt:            time.Now().Truncate(time.Microsecond).UTC(),
	}

	data, err := json.Marshal(&original)
	require.NoError(t, err)

	var decoded Epoch
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// ApplicationID is json:"-" (DB-only FK), intentionally lost in JSON roundtrip.
	require.Zero(t, decoded.ApplicationID)
	require.Equal(t, original.Index, decoded.Index)
	require.Equal(t, original.FirstBlock, decoded.FirstBlock)
	require.Equal(t, original.LastBlock, decoded.LastBlock)
	require.Equal(t, original.InputIndexLowerBound, decoded.InputIndexLowerBound)
	require.Equal(t, original.InputIndexUpperBound, decoded.InputIndexUpperBound)
	require.Equal(t, original.VirtualIndex, decoded.VirtualIndex)
	require.Equal(t, original.Status, decoded.Status)
	require.Equal(t, original.OutputsMerkleRoot, decoded.OutputsMerkleRoot)
}

func TestInputJSONRoundtrip(t *testing.T) {
	machineHash := common.HexToHash("0x1234")
	original := Input{
		EpochApplicationID: 1,
		EpochIndex:         3,
		Index:              7,
		BlockNumber:        12345,
		RawData:            []byte{0xde, 0xad, 0xbe, 0xef},
		Status:             InputCompletionStatus_Accepted,
		MachineHash:        &machineHash,
		CreatedAt:          time.Now().Truncate(time.Microsecond).UTC(),
		UpdatedAt:          time.Now().Truncate(time.Microsecond).UTC(),
	}

	data, err := json.Marshal(&original)
	require.NoError(t, err)

	var decoded Input
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// EpochApplicationID is json:"-" (DB-only FK), intentionally lost in JSON roundtrip.
	require.Zero(t, decoded.EpochApplicationID)
	require.Equal(t, original.EpochIndex, decoded.EpochIndex)
	require.Equal(t, original.Index, decoded.Index)
	require.Equal(t, original.BlockNumber, decoded.BlockNumber)
	require.Equal(t, original.RawData, decoded.RawData)
	require.Equal(t, original.Status, decoded.Status)
	require.Equal(t, original.MachineHash, decoded.MachineHash)
}

func TestOutputJSONRoundtrip(t *testing.T) {
	hash := common.HexToHash("0xaaaa")
	txHash := common.HexToHash("0xbbbb")
	original := Output{
		InputEpochApplicationID:  1,
		EpochIndex:               2,
		InputIndex:               5,
		Index:                    10,
		RawData:                  []byte{0xca, 0xfe},
		Hash:                     &hash,
		OutputHashesSiblings:     []common.Hash{common.HexToHash("0x1111")},
		ExecutionTransactionHash: &txHash,
		CreatedAt:                time.Now().Truncate(time.Microsecond).UTC(),
		UpdatedAt:                time.Now().Truncate(time.Microsecond).UTC(),
	}

	data, err := json.Marshal(&original)
	require.NoError(t, err)

	var decoded Output
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// InputEpochApplicationID is json:"-" (DB-only FK), intentionally lost in JSON roundtrip.
	require.Zero(t, decoded.InputEpochApplicationID)
	require.Equal(t, original.EpochIndex, decoded.EpochIndex)
	require.Equal(t, original.InputIndex, decoded.InputIndex)
	require.Equal(t, original.Index, decoded.Index)
	require.Equal(t, original.RawData, decoded.RawData)
	require.Equal(t, original.Hash, decoded.Hash)
	require.Equal(t, original.OutputHashesSiblings, decoded.OutputHashesSiblings)
	require.Equal(t, original.ExecutionTransactionHash, decoded.ExecutionTransactionHash)
}

func TestReportJSONRoundtrip(t *testing.T) {
	original := Report{
		InputEpochApplicationID: 1,
		EpochIndex:              4,
		InputIndex:              8,
		Index:                   0,
		RawData:                 []byte{0x01, 0x02, 0x03},
		CreatedAt:               time.Now().Truncate(time.Microsecond).UTC(),
		UpdatedAt:               time.Now().Truncate(time.Microsecond).UTC(),
	}

	data, err := json.Marshal(&original)
	require.NoError(t, err)

	var decoded Report
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// InputEpochApplicationID is json:"-" (DB-only FK), intentionally lost in JSON roundtrip.
	require.Zero(t, decoded.InputEpochApplicationID)
	require.Equal(t, original.EpochIndex, decoded.EpochIndex)
	require.Equal(t, original.InputIndex, decoded.InputIndex)
	require.Equal(t, original.Index, decoded.Index)
	require.Equal(t, original.RawData, decoded.RawData)
}

func TestTournamentJSONRoundtrip(t *testing.T) {
	parentAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	parentMatch := common.HexToHash("0xeeee")
	winner := common.HexToHash("0xdddd")
	finalState := common.HexToHash("0xcccc")
	original := Tournament{
		ApplicationID:           1,
		EpochIndex:              3,
		Address:                 common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		ParentTournamentAddress: &parentAddr,
		ParentMatchIDHash:       &parentMatch,
		MaxLevel:                4,
		Level:                   2,
		Log2Step:                16,
		Height:                  8,
		WinnerCommitment:        &winner,
		FinalStateHash:          &finalState,
		FinishedAtBlock:         9999,
		CreatedAt:               time.Now().Truncate(time.Microsecond).UTC(),
		UpdatedAt:               time.Now().Truncate(time.Microsecond).UTC(),
	}

	data, err := json.Marshal(&original)
	require.NoError(t, err)

	var decoded Tournament
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// ApplicationID is json:"-" (DB-only FK), intentionally lost in JSON roundtrip.
	require.Zero(t, decoded.ApplicationID)
	require.Equal(t, original.EpochIndex, decoded.EpochIndex)
	require.Equal(t, original.Address, decoded.Address)
	require.Equal(t, original.ParentTournamentAddress, decoded.ParentTournamentAddress)
	require.Equal(t, original.ParentMatchIDHash, decoded.ParentMatchIDHash)
	require.Equal(t, original.MaxLevel, decoded.MaxLevel)
	require.Equal(t, original.Level, decoded.Level)
	require.Equal(t, original.Log2Step, decoded.Log2Step)
	require.Equal(t, original.Height, decoded.Height)
	require.Equal(t, original.WinnerCommitment, decoded.WinnerCommitment)
	require.Equal(t, original.FinalStateHash, decoded.FinalStateHash)
	require.Equal(t, original.FinishedAtBlock, decoded.FinishedAtBlock)
}

func TestCommitmentJSONRoundtrip(t *testing.T) {
	original := Commitment{
		ApplicationID:     1,
		EpochIndex:        7,
		TournamentAddress: common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		Commitment:        common.HexToHash("0x5555"),
		FinalStateHash:    common.HexToHash("0x6666"),
		SubmitterAddress:  common.HexToAddress("0x1111111111111111111111111111111111111111"),
		BlockNumber:       54321,
		TxHash:            common.HexToHash("0x7777"),
		CreatedAt:         time.Now().Truncate(time.Microsecond).UTC(),
		UpdatedAt:         time.Now().Truncate(time.Microsecond).UTC(),
	}

	data, err := json.Marshal(&original)
	require.NoError(t, err)

	var decoded Commitment
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// ApplicationID is json:"-" (DB-only FK), intentionally lost in JSON roundtrip.
	require.Zero(t, decoded.ApplicationID)
	require.Equal(t, original.EpochIndex, decoded.EpochIndex)
	require.Equal(t, original.TournamentAddress, decoded.TournamentAddress)
	require.Equal(t, original.Commitment, decoded.Commitment)
	require.Equal(t, original.FinalStateHash, decoded.FinalStateHash)
	require.Equal(t, original.SubmitterAddress, decoded.SubmitterAddress)
	require.Equal(t, original.BlockNumber, decoded.BlockNumber)
	require.Equal(t, original.TxHash, decoded.TxHash)
}

func TestOutputJSONRoundtripZeroValues(t *testing.T) {
	original := Output{
		EpochIndex: 0,
		InputIndex: 0,
		Index:      0,
		RawData:    []byte{},
	}

	data, err := json.Marshal(&original)
	require.NoError(t, err)

	var decoded Output
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, original.EpochIndex, decoded.EpochIndex)
	require.Equal(t, original.InputIndex, decoded.InputIndex)
	require.Equal(t, original.Index, decoded.Index)
}

func TestOutputUnmarshalJSONInvalidHex(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr string
	}{
		{
			name:    "invalid EpochIndex",
			json:    `{"epoch_index":"bad","input_index":"0x0","index":"0x0","raw_data":"0x"}`,
			wantErr: "EpochIndex",
		},
		{
			name:    "invalid InputIndex",
			json:    `{"epoch_index":"0x0","input_index":"bad","index":"0x0","raw_data":"0x"}`,
			wantErr: "InputIndex",
		},
		{
			name:    "invalid RawData",
			json:    `{"epoch_index":"0x0","input_index":"0x0","index":"0x0","raw_data":"not-hex"}`,
			wantErr: "RawData",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output Output
			err := json.Unmarshal([]byte(tt.json), &output)
			require.Error(t, err)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestReportUnmarshalJSONInvalidHex(t *testing.T) {
	invalidJSON := `{"epoch_index":"0x0","input_index":"bad","index":"0x0","raw_data":"0x"}`
	var report Report
	err := json.Unmarshal([]byte(invalidJSON), &report)
	require.Error(t, err)
	require.ErrorContains(t, err, "InputIndex")
}
