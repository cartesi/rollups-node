// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package api

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"reflect"

	"github.com/cartesi/rollups-node/internal/jsonutil"
)

type StringOrList []string

func (s *StringOrList) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value, jsonutil.Options); err != nil {
			return err
		}
		*s = []string{value}
		return nil
	}

	var values []string
	if err := json.Unmarshal(data, &values, jsonutil.Options); err != nil {
		return fmt.Errorf("expected a string or an array of strings: %w", err)
	}
	*s = values
	return nil
}

// ListApplicationsParams aligns with the OpenRPC specification
type ListApplicationsParams struct {
	Limit      uint64 `json:"limit"`
	Offset     uint64 `json:"offset"`
	Descending bool   `json:"descending,omitzero"`
}

// GetApplicationParams aligns with the OpenRPC specification
type GetApplicationParams struct {
	Application string `json:"application"`
}

// ListEpochsParams aligns with the OpenRPC specification
type ListEpochsParams struct {
	Application string        `json:"application"`
	Status      *StringOrList `json:"status,omitzero"`
	Limit       uint64        `json:"limit"`
	Offset      uint64        `json:"offset"`
	Descending  bool          `json:"descending,omitzero"`
	From        *string       `json:"from,omitzero"` // inclusive lower bound on the epoch index (hex)
	To          *string       `json:"to,omitzero"`   // inclusive upper bound on the epoch index (hex)
}

// GetEpochParams aligns with the OpenRPC specification
type GetEpochParams struct {
	Application string `json:"application"`
	EpochIndex  string `json:"epoch_index"`
}

// GetEpochByVirtualIndexParams aligns with the OpenRPC specification
type GetEpochByVirtualIndexParams struct {
	Application  string `json:"application"`
	VirtualIndex string `json:"virtual_index"`
}

// GetLastAcceptedEpochIndexParams with the OpenRPC specification
type GetLastAcceptedEpochIndexParams struct {
	Application string `json:"application"`
}

// ListInputsParams aligns with the OpenRPC specification
type ListInputsParams struct {
	Application     string  `json:"application"`
	EpochIndex      *string `json:"epoch_index,omitzero"`
	Sender          *string `json:"sender,omitzero"`
	TransactionHash *string `json:"transaction_hash,omitzero"`
	Limit           uint64  `json:"limit"`
	Offset          uint64  `json:"offset"`
	Descending      bool    `json:"descending,omitzero"`
	From            *string `json:"from,omitzero"` // inclusive lower bound on the input index (hex)
	To              *string `json:"to,omitzero"`   // inclusive upper bound on the input index (hex)
}

// GetInputParams aligns with the OpenRPC specification
type GetInputParams struct {
	Application string `json:"application"`
	InputIndex  string `json:"input_index"`
}

// GetProcessedInputCountParams aligns with the OpenRPC specification
type GetProcessedInputCountParams struct {
	Application string `json:"application"`
}

// ListOutputsParams aligns with the OpenRPC specification
type ListOutputsParams struct {
	Application    string        `json:"application"`
	EpochIndex     *string       `json:"epoch_index,omitzero"`
	InputIndex     *string       `json:"input_index,omitzero"`
	OutputType     *StringOrList `json:"output_type,omitzero"`
	VoucherAddress *string       `json:"voucher_address,omitzero"`
	Limit          uint64        `json:"limit"`
	Offset         uint64        `json:"offset"`
	Descending     bool          `json:"descending,omitzero"`
	From           *string       `json:"from,omitzero"` // inclusive lower bound on the output index (hex)
	To             *string       `json:"to,omitzero"`   // inclusive upper bound on the output index (hex)
	Executed       *bool         `json:"executed,omitzero"`
}

// GetOutputParams aligns with the OpenRPC specification
type GetOutputParams struct {
	Application string `json:"application"`
	OutputIndex string `json:"output_index"`
}

// ListReportsParams aligns with the OpenRPC specification
type ListReportsParams struct {
	Application string  `json:"application"`
	EpochIndex  *string `json:"epoch_index,omitzero"`
	InputIndex  *string `json:"input_index,omitzero"`
	Limit       uint64  `json:"limit"`
	Offset      uint64  `json:"offset"`
	Descending  bool    `json:"descending,omitzero"`
	From        *string `json:"from,omitzero"` // inclusive lower bound on the report index (hex)
	To          *string `json:"to,omitzero"`   // inclusive upper bound on the report index (hex)
}

// GetReportParams aligns with the OpenRPC specification
type GetReportParams struct {
	Application string `json:"application"`
	ReportIndex string `json:"report_index"`
}

// ListTournamentsParams aligns with the OpenRPC specification
type ListTournamentsParams struct {
	Application             string  `json:"application"`
	EpochIndex              *string `json:"epoch_index,omitzero"`
	Level                   *string `json:"level,omitzero"`
	ParentTournamentAddress *string `json:"parent_tournament_address,omitzero"`
	ParentMatchIDHash       *string `json:"parent_match_id_hash,omitzero"`
	Limit                   uint64  `json:"limit"`
	Offset                  uint64  `json:"offset"`
	Descending              bool    `json:"descending,omitzero"`
}

// GetTournamentParams aligns with the OpenRPC specification
type GetTournamentParams struct {
	Application string `json:"application"`
	Address     string `json:"address"`
}

// ListCommitmentsParams aligns with the OpenRPC specification
type ListCommitmentsParams struct {
	Application       string  `json:"application"`
	EpochIndex        *string `json:"epoch_index,omitzero"`
	TournamentAddress *string `json:"tournament_address,omitzero"`
	Limit             uint64  `json:"limit"`
	Offset            uint64  `json:"offset"`
	Descending        bool    `json:"descending,omitzero"`
}

// GetCommitmentParams aligns with the OpenRPC specification
type GetCommitmentParams struct {
	Application       string `json:"application"`
	EpochIndex        string `json:"epoch_index"`
	TournamentAddress string `json:"tournament_address"`
	Commitment        string `json:"commitment"`
}

// ListMatchesParams aligns with the OpenRPC specification
type ListMatchesParams struct {
	Application       string  `json:"application"`
	EpochIndex        *string `json:"epoch_index,omitzero"`
	TournamentAddress *string `json:"tournament_address,omitzero"`
	Limit             uint64  `json:"limit"`
	Offset            uint64  `json:"offset"`
	Descending        bool    `json:"descending,omitzero"`
}

// GetMatchParams aligns with the OpenRPC specification
type GetMatchParams struct {
	Application       string `json:"application"`
	EpochIndex        string `json:"epoch_index"`
	TournamentAddress string `json:"tournament_address"`
	IDHash            string `json:"id_hash"`
}

// ListMatchAdvancesParams aligns with the OpenRPC specification
type ListMatchAdvancesParams struct {
	Application       string `json:"application"`
	EpochIndex        string `json:"epoch_index"`
	TournamentAddress string `json:"tournament_address"`
	IDHash            string `json:"id_hash"`
	Limit             uint64 `json:"limit"`
	Offset            uint64 `json:"offset"`
	Descending        bool   `json:"descending,omitzero"`
}

// GetMatchAdvanceParams aligns with the OpenRPC specification
type GetMatchAdvanceParams struct {
	Application       string `json:"application"`
	EpochIndex        string `json:"epoch_index"`
	TournamentAddress string `json:"tournament_address"`
	IDHash            string `json:"id_hash"`
	TxHash            string `json:"tx_hash"`
	LogIndex          string `json:"log_index"`
}

// ListBondEventsParams selects immutable refund and recovery events.
type ListBondEventsParams struct {
	Application       string  `json:"application"`
	EpochIndex        *string `json:"epoch_index,omitzero"`
	TournamentAddress *string `json:"tournament_address,omitzero"`
	Limit             uint64  `json:"limit"`
	Offset            uint64  `json:"offset"`
	Descending        bool    `json:"descending,omitzero"`
}

// GetBondEventParams identifies an event by its transaction and block log index.
type GetBondEventParams struct {
	Application string `json:"application"`
	TxHash      string `json:"tx_hash"`
	LogIndex    string `json:"log_index"`
}

// ListStandingChangesParams selects standing history within one tournament.
type ListStandingChangesParams struct {
	Application       string `json:"application"`
	EpochIndex        string `json:"epoch_index"`
	TournamentAddress string `json:"tournament_address"`
	Limit             uint64 `json:"limit"`
	Offset            uint64 `json:"offset"`
	Descending        bool   `json:"descending,omitempty"`
}

// GetStandingChangeParams identifies a scoped event by transaction and block log index.
type GetStandingChangeParams struct {
	Application       string `json:"application"`
	EpochIndex        string `json:"epoch_index"`
	TournamentAddress string `json:"tournament_address"`
	TxHash            string `json:"tx_hash"`
	LogIndex          string `json:"log_index"`
}

// ListWithdrawalsParams aligns with the OpenRPC specification
type ListWithdrawalsParams struct {
	Application  string  `json:"application"`
	AccountIndex *string `json:"account_index,omitzero"`
	Limit        uint64  `json:"limit"`
	Offset       uint64  `json:"offset"`
	Descending   bool    `json:"descending,omitzero"`
}

// GetWithdrawalParams aligns with the OpenRPC specification
type GetWithdrawalParams struct {
	Application  string `json:"application"`
	AccountIndex string `json:"account_index"`
}

// UnmarshalParams supports both by-name (object) and by-position (array) parameter structures.
// If params is an object, it simply does json.Unmarshal; if it's an array, it will attempt
// to unmarshal each positional parameter into the target struct field in declaration order.
func UnmarshalParams(data jsontext.Value, target any) error {
	data = bytes.TrimSpace(data)
	switch {
	case len(data) == 0 || bytes.Equal(data, []byte("null")):
		// Parameters field is absent
		return nil
	case data[0] == '[':
		// Unmarshal positional parameters into a slice of jsontext.Value.
		var rawParams []jsontext.Value
		if err := json.Unmarshal(data, &rawParams, jsonutil.Options); err != nil {
			return err
		}
		// Use reflection to set values in the target struct in the order they appear.
		val := reflect.ValueOf(target)
		if val.Kind() != reflect.Pointer || val.IsNil() {
			return fmt.Errorf("error unmarshalling positional parameters target must be a non-nil pointer to a struct")
		}
		val = val.Elem()
		if val.Kind() != reflect.Struct {
			return fmt.Errorf("error unmarshalling positional parameters target must point to a struct")
		}
		typ := val.Type()
		fields := make([]int, 0, typ.NumField())
		for i := 0; i < typ.NumField(); i++ {
			if typ.Field(i).Tag.Get("json") != "-" {
				fields = append(fields, i)
			}
		}
		if len(rawParams) > len(fields) {
			return fmt.Errorf("error unmarshalling positional parameters, expected %d params, got %d",
				len(fields), len(rawParams))
		}
		// Map positional parameters to JSON-visible fields in declaration order.
		for i, fieldIndex := range fields {
			if i >= len(rawParams) {
				break
			}
			field := val.Field(fieldIndex)
			if !field.CanSet() {
				return fmt.Errorf("error unmarshalling positional parameter field %q is not settable",
					typ.Field(fieldIndex).Name)
			}
			// Unmarshal the corresponding raw parameter into the field.
			if err := json.Unmarshal(rawParams[i], field.Addr().Interface(), jsonutil.Options); err != nil {
				return fmt.Errorf("error unmarshalling positional parameter %d for field %s: %w",
					i, typ.Field(fieldIndex).Name, err)
			}
		}
		return nil
	default:
		// Otherwise, assume by-name structure.
		return json.Unmarshal(data, target, jsonutil.Options)
	}
}
