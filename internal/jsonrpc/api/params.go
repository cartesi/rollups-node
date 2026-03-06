// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package api

// ListApplicationsParams aligns with the OpenRPC specification
type ListApplicationsParams struct {
	Limit      uint64 `json:"limit"`
	Offset     uint64 `json:"offset"`
	Descending bool   `json:"descending,omitempty"`
}

// GetApplicationParams aligns with the OpenRPC specification
type GetApplicationParams struct {
	Application string `json:"application"`
}

// ListEpochsParams aligns with the OpenRPC specification
type ListEpochsParams struct {
	Application string  `json:"application"`
	Status      *string `json:"status,omitempty"`
	Limit       uint64  `json:"limit"`
	Offset      uint64  `json:"offset"`
	Descending  bool    `json:"descending,omitempty"`
}

// GetEpochParams aligns with the OpenRPC specification
type GetEpochParams struct {
	Application string `json:"application"`
	EpochIndex  string `json:"epoch_index"`
}

// GetLastAcceptedEpochIndexParams with the OpenRPC specification
type GetLastAcceptedEpochIndexParams struct {
	Application string `json:"application"`
}

// ListInputsParams aligns with the OpenRPC specification
type ListInputsParams struct {
	Application string  `json:"application"`
	EpochIndex  *string `json:"epoch_index,omitempty"`
	Sender      *string `json:"sender,omitempty"`
	Limit       uint64  `json:"limit"`
	Offset      uint64  `json:"offset"`
	Descending  bool    `json:"descending,omitempty"`
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
	Application    string  `json:"application"`
	EpochIndex     *string `json:"epoch_index,omitempty"`
	InputIndex     *string `json:"input_index,omitempty"`
	OutputType     *string `json:"output_type,omitempty"`
	VoucherAddress *string `json:"voucher_address,omitempty"`
	Limit          uint64  `json:"limit"`
	Offset         uint64  `json:"offset"`
	Descending     bool    `json:"descending,omitempty"`
}

// GetOutputParams aligns with the OpenRPC specification
type GetOutputParams struct {
	Application string `json:"application"`
	OutputIndex string `json:"output_index"`
}

// ListReportsParams aligns with the OpenRPC specification
type ListReportsParams struct {
	Application string  `json:"application"`
	EpochIndex  *string `json:"epoch_index,omitempty"`
	InputIndex  *string `json:"input_index,omitempty"`
	Limit       uint64  `json:"limit"`
	Offset      uint64  `json:"offset"`
	Descending  bool    `json:"descending,omitempty"`
}

// GetReportParams aligns with the OpenRPC specification
type GetReportParams struct {
	Application string `json:"application"`
	ReportIndex string `json:"report_index"`
}

// ListTournamentsParams aligns with the OpenRPC specification
type ListTournamentsParams struct {
	Application             string  `json:"application"`
	EpochIndex              *string `json:"epoch_index,omitempty"`
	Level                   *string `json:"level,omitempty"`
	ParentTournamentAddress *string `json:"parent_tournament_address,omitempty"`
	ParentMatchIDHash       *string `json:"parent_match_id_hash,omitempty"`
	Limit                   uint64  `json:"limit"`
	Offset                  uint64  `json:"offset"`
	Descending              bool    `json:"descending,omitempty"`
}

// GetTournamentParams aligns with the OpenRPC specification
type GetTournamentParams struct {
	Application string `json:"application"`
	Address     string `json:"address"`
}

// ListCommitmentsParams aligns with the OpenRPC specification
type ListCommitmentsParams struct {
	Application       string  `json:"application"`
	EpochIndex        *string `json:"epoch_index,omitempty"`
	TournamentAddress *string `json:"tournament_address,omitempty"`
	Limit             uint64  `json:"limit"`
	Offset            uint64  `json:"offset"`
	Descending        bool    `json:"descending,omitempty"`
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
	EpochIndex        *string `json:"epoch_index,omitempty"`
	TournamentAddress *string `json:"tournament_address,omitempty"`
	Limit             uint64  `json:"limit"`
	Offset            uint64  `json:"offset"`
	Descending        bool    `json:"descending,omitempty"`
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
	Descending        bool   `json:"descending,omitempty"`
}

// GetMatchAdvancedParams aligns with the OpenRPC specification
type GetMatchAdvancedParams struct {
	Application       string `json:"application"`
	EpochIndex        string `json:"epoch_index"`
	TournamentAddress string `json:"tournament_address"`
	IDHash            string `json:"id_hash"`
	Parent            string `json:"parent"`
}
