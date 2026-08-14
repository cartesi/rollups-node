// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package replay

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"

	"github.com/cartesi/rollups-node/internal/model"
	"github.com/cartesi/rollups-node/internal/repository"
	"github.com/ethereum/go-ethereum/common"
)

type contradiction func(field string, expected, actual any) error

func newContradiction(
	application string,
	epochIndex *uint64,
	inputIndex uint64,
	field string,
	expected, actual any,
) *ContradictionError {
	if application == "" {
		application = "<unknown>"
	}
	return &ContradictionError{
		Application: application,
		EpochIndex:  epochIndex,
		InputIndex:  inputIndex,
		Field:       field,
		Expected:    fmt.Sprint(expected),
		Actual:      fmt.Sprint(actual),
	}
}

func knownEpochIndex(epochIndex uint64) *uint64 { return &epochIndex }

func compareRecord(
	application string,
	applicationID int64,
	isPRT bool,
	verification repository.ReplayVerificationLevel,
	record *model.ReplayRecord,
	actual *model.AdvanceResult,
) error {
	if record == nil {
		return newContradiction(application, nil, 0, "record", "persisted replay record", "nil")
	}
	contradiction := func(field string, expected, got any) error {
		return newContradiction(
			application,
			knownEpochIndex(record.Input.EpochIndex),
			record.Input.InputIndex,
			field,
			expected,
			got,
		)
	}
	if actual == nil {
		return contradiction("result", "completed result", "nil")
	}
	if record.Input.ApplicationID != applicationID {
		return contradiction("application_id", applicationID, record.Input.ApplicationID)
	}
	if actual.EpochIndex != record.Input.EpochIndex {
		return contradiction("epoch_index", record.Input.EpochIndex, actual.EpochIndex)
	}
	if actual.InputIndex != record.Input.InputIndex {
		return contradiction("input_index", record.Input.InputIndex, actual.InputIndex)
	}
	if actual.Status != record.Input.Status {
		return contradiction("status", record.Input.Status, actual.Status)
	}
	if !record.Input.Status.IsCompleted() {
		return contradiction("status", "completed status", record.Input.Status)
	}
	if err := compareExceptionData(record.Input, actual, contradiction); err != nil {
		return err
	}
	if record.Input.MachineHash == nil {
		return contradiction("machine_hash", "persisted hash", "missing")
	}
	if actual.MachineHash != *record.Input.MachineHash {
		return contradiction("machine_hash", record.Input.MachineHash.Hex(), actual.MachineHash.Hex())
	}
	if record.Input.OutputsHash == nil {
		return contradiction("outputs_hash", "persisted hash", "missing")
	}
	if actual.OutputsHash != *record.Input.OutputsHash {
		return contradiction("outputs_hash", record.Input.OutputsHash.Hex(), actual.OutputsHash.Hex())
	}
	if verification == repository.ReplayVerificationCanonical {
		return nil
	}

	if record.Input.Status == model.InputCompletionStatus_Accepted {
		if err := compareBytes("outputs", record.Outputs, actual.Outputs, contradiction); err != nil {
			return err
		}
		if err := compareBytes("reports", record.Reports, actual.Reports, contradiction); err != nil {
			return err
		}
	} else {
		// Effects of nonaccepted executions are diagnostics, not canonical.
		if len(record.Outputs) != 0 {
			return contradiction("outputs.count", 0, len(record.Outputs))
		}
		if len(record.Reports) != 0 {
			return contradiction("reports.count", 0, len(record.Reports))
		}
	}

	if !isPRT {
		if len(record.StateHashes) != 0 {
			return contradiction("state_hashes.count", 0, len(record.StateHashes))
		}
		if len(actual.PeriodicStateHashes) != 0 || actual.PaddingRepetitions != 0 {
			return contradiction(
				"replay_hash_collection",
				"none",
				fmt.Sprintf("hashes=%d padding=%d", len(actual.PeriodicStateHashes), actual.PaddingRepetitions),
			)
		}
		return nil
	}
	return compareHashCollection(record, actual, contradiction)
}

func compareExceptionData(
	input model.ReplayInput,
	actual *model.AdvanceResult,
	contradiction contradiction,
) error {
	if input.Status == model.InputCompletionStatus_Exception {
		if input.ExceptionData == nil {
			return contradiction("exception_data", "persisted payload", "missing")
		}
		if actual.ExceptionData == nil {
			return contradiction("exception_data", compactBytes(input.ExceptionData), "missing")
		}
		if !bytes.Equal(input.ExceptionData, actual.ExceptionData) {
			return contradiction(
				"exception_data",
				compactBytes(input.ExceptionData),
				compactBytes(actual.ExceptionData),
			)
		}
		return nil
	}
	if input.ExceptionData != nil {
		return contradiction("exception_data", "none", compactBytes(input.ExceptionData))
	}
	if actual.ExceptionData != nil {
		return contradiction("exception_data", "none", compactBytes(actual.ExceptionData))
	}
	return nil
}

func compareBytes(field string, expected, actual [][]byte, contradiction contradiction) error {
	if len(expected) != len(actual) {
		return contradiction(field+".count", len(expected), len(actual))
	}
	for i := range expected {
		if !bytes.Equal(expected[i], actual[i]) {
			return contradiction(
				fmt.Sprintf("%s[%d]", field, i),
				compactBytes(expected[i]),
				compactBytes(actual[i]),
			)
		}
	}
	return nil
}

func compactBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return fmt.Sprintf("len=%d sha256=%x", len(data), digest[:8])
}

func compareHashCollection(
	record *model.ReplayRecord,
	actual *model.AdvanceResult,
	contradiction contradiction,
) error {
	if err := model.ValidateInputHashCollectionSpan(
		uint64(len(actual.PeriodicStateHashes)),
		actual.PaddingRepetitions,
	); err != nil {
		return contradiction(
			"state_hashes.span",
			model.InputHashCollectionCapacity,
			fmt.Sprintf("hashes=%d padding=%d", len(actual.PeriodicStateHashes), actual.PaddingRepetitions),
		)
	}
	if actual.PaddingRepetitions == 0 {
		return contradiction("state_hashes.final.repetitions", ">0", 0)
	}
	actualCount := len(actual.PeriodicStateHashes) + 1
	if len(record.StateHashes) != actualCount {
		return contradiction("state_hashes.count", len(record.StateHashes), actualCount)
	}
	for i := 1; i < len(record.StateHashes); i++ {
		previousIndex := record.StateHashes[i-1].Index
		if previousIndex == math.MaxUint64 {
			return contradiction("state_hashes.index", "index after uint64 maximum", record.StateHashes[i].Index)
		}
		if record.StateHashes[i].Index != previousIndex+1 {
			return contradiction("state_hashes.index", previousIndex+1, record.StateHashes[i].Index)
		}
	}
	for i, hash := range actual.PeriodicStateHashes {
		row := record.StateHashes[i]
		if row.Repetitions != 1 {
			return contradiction(fmt.Sprintf("state_hashes[%d].repetitions", i), 1, row.Repetitions)
		}
		if row.MachineHash != common.Hash(hash) {
			return contradiction(
				fmt.Sprintf("state_hashes[%d].machine_hash", i),
				row.MachineHash.Hex(),
				common.Hash(hash).Hex(),
			)
		}
	}
	final := record.StateHashes[len(record.StateHashes)-1]
	if final.MachineHash != *record.Input.MachineHash {
		return contradiction("state_hashes.final.machine_hash", record.Input.MachineHash.Hex(), final.MachineHash.Hex())
	}
	if final.MachineHash != actual.MachineHash {
		return contradiction("state_hashes.final.replay_hash", final.MachineHash.Hex(), actual.MachineHash.Hex())
	}
	if final.Repetitions != actual.PaddingRepetitions {
		return contradiction("state_hashes.final.repetitions", final.Repetitions, actual.PaddingRepetitions)
	}
	return nil
}
